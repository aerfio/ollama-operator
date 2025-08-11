package model

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/go-logr/logr/testr"
	"github.com/google/go-cmp/cmp"
	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/ollamaclient"
	"aerf.io/ollama-operator/internal/testutils"
)

func Test_isStatefulSetReady(t *testing.T) {
	tests := []struct {
		name        string
		sts         *appsv1.StatefulSet
		ready       bool
		msgContains string
	}{
		{
			name: "sts not ready - too few ready replicas",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 123,
				},
				Spec: appsv1.StatefulSetSpec{
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
					},
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 123,
					Replicas:           3,
					ReadyReplicas:      1,
				},
			},
			ready:       false,
			msgContains: "Waiting for 2 pods",
		},
		{
			name: "sts ready",
			sts: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 123,
				},
				Spec: appsv1.StatefulSetSpec{
					UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
						Type: appsv1.RollingUpdateStatefulSetStrategyType,
					},
					Replicas: ptr.To(int32(3)),
				},
				Status: appsv1.StatefulSetStatus{
					ObservedGeneration: 123,
					Replicas:           3,
					ReadyReplicas:      3,
				},
			},
			ready:       true,
			msgContains: "rolling update complete",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ready, err := isStatefulSetReady(tt.sts)
			require.NoError(t, err) // should never err
			assert.Equal(t, tt.ready, ready, msg)
			if tt.msgContains != "" {
				assert.Contains(t, msg, tt.msgContains)
			}
		})
	}
}

// test apiserver does not return GVK inside the struct, what the hell
type addGVKReconciler struct {
	inner reconcile.ObjectReconciler[*ollamav1alpha1.Model]
}

func (a addGVKReconciler) Reconcile(ctx context.Context, object *ollamav1alpha1.Model) (reconcile.Result, error) {
	object.SetGroupVersionKind(ollamav1alpha1.ModelGroupVersionKind)
	return a.inner.Reconcile(ctx, object)
}

func TestReconciler_Reconcile(t *testing.T) {
	require.NoError(t, ollamav1alpha1.AddToScheme(scheme.Scheme))
	testEnv := envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths:     []string{testutils.GetCRDsDir(t)},
	}
	restCfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testEnv.Stop())
	})
	cli, err := client.New(restCfg, client.Options{})
	require.NoError(t, err)

	clientset, err := kubernetes.NewForConfig(restCfg)
	require.NoError(t, err)
	_ = clientset.UseLegacyDiscovery

	t.Run("Model controllers correctly sets the status conditions to signal that model is being pulled", func(t *testing.T) {
		model := &ollamav1alpha1.Model{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: ollamav1alpha1.ModelSpec{
				Model: "gemma2:2b",
			},
		}
		require.NoError(t, cli.Create(context.Background(), model))

		listCallNumber := 0
		showCallNumber := 0
		r := reconcile.AsReconciler[*ollamav1alpha1.Model](cli,
			&addGVKReconciler{
				inner: &Reconciler{
					client:         client.WithFieldOwner(cli, "ollama-operator"),
					recorder:       record.NewFakeRecorder(1000),
					baseHTTPClient: &http.Client{},
					tp:             noop.NewTracerProvider(),
					ollamaClientProvider: &ollamaclient.TestOllamaClientProvider{
						Client: &ollamaclient.TestOllamaClient{
							OnList: func(ctx context.Context) (*api.ListResponse, error) {
								defer func() {
									listCallNumber += 1
								}()
								switch listCallNumber {
								case 0:
									// simulate random error
									return nil, fmt.Errorf("boom")
								case 1:
									// returned first when the model is not yet pulled
									return &api.ListResponse{}, nil
								default:
									return &api.ListResponse{
										Models: []api.ListModelResponse{
											{
												Model: model.Spec.Model,
											},
										},
									}, nil
								}
							},
							OnShow: func(ctx context.Context, req *api.ShowRequest) (*api.ShowResponse, error) {
								defer func() {
									showCallNumber += 1
								}()
								switch showCallNumber {
								case 0:
									return nil, fmt.Errorf("show: boom")
								default:
									return &api.ShowResponse{
										Details: api.ModelDetails{
											ParentModel:       "pm",
											Format:            "f",
											Family:            "f",
											Families:          []string{"fam"},
											ParameterSize:     "ps",
											QuantizationLevel: "ql",
										},
									}, nil
								}
							},
						},
					},
				},
			},
		)
		reconcileFn := func() error {
			_, err := r.Reconcile(ctrl.LoggerInto(context.Background(), testr.NewWithOptions(t, testr.Options{Verbosity: 10})), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: model.GetNamespace(),
					Name:      model.GetName(),
				},
			})
			return err
		}

		err = reconcileFn()
		require.NoError(t, err)

		getModel := func() *ollamav1alpha1.Model {
			m := &ollamav1alpha1.Model{}
			err := cli.Get(context.Background(), client.ObjectKey{Name: model.GetName(), Namespace: model.GetNamespace()}, m)
			require.NoError(t, err)
			return m
		}

		mdl := getModel()
		t.Logf("status: %#v", mdl.Status)

		syncedCondition := mdl.GetCondition(xpv1.TypeSynced)
		require.Equalf(t, corev1.ConditionTrue, syncedCondition.Status, "Synced condition has status True: %#v", syncedCondition)

		readyCondition := mdl.GetCondition(xpv1.TypeReady)
		require.Equalf(t, corev1.ConditionFalse, readyCondition.Status, "Ready condition has status False: %#v", readyCondition)

		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			sts := &appsv1.StatefulSet{}
			err = cli.Get(context.Background(), client.ObjectKey{Name: model.GetName(), Namespace: model.GetNamespace()}, sts)
			if err != nil {
				return err
			}
			sts.Status.Replicas = 1
			sts.Status.ReadyReplicas = 1
			sts.Status.CurrentReplicas = 1
			sts.Status.UpdatedReplicas = 1
			sts.Status.ObservedGeneration = sts.Generation
			sts.Status.AvailableReplicas = 1
			return cli.Status().Update(context.Background(), sts)
		})
		require.NoError(t, err)
		err = reconcileFn()
		require.Error(t, err, "error must be non-nil: there should be an error from failed list call")

		mdl = getModel()
		t.Logf("status: %#v", mdl.Status)
		if diff := cmp.Diff(mdl.GetCondition(xpv1.TypeSynced), xpv1.ReconcileError(errors.New("failed to list local models: boom")), testutils.IgnoreXPv1ConditionFields()); diff != "" {
			t.Fatalf("conditions differ, -got +want:\n%s", diff)
		}

		err = reconcileFn()
		require.NoError(t, err)
		mdl = getModel()
		readyCondition = mdl.GetCondition(xpv1.TypeReady)
		readyCondition.Message = "" // I have no idea why I have to override this msg if I'm ignoring this field, maybe cmpopts doesnt like ``
		if diff := cmp.Diff(readyCondition, xpv1.Creating(), testutils.IgnoreXPv1ConditionFields("Message")); diff != "" {
			t.Fatalf("conditions differ, -got +want:\n%s", diff)
		}

		err = reconcileFn()
		require.Error(t, err)
		mdl = getModel()
		require.Contains(t, mdl.GetCondition(xpv1.TypeSynced).Message, "show: boom")
		require.Equal(t, corev1.ConditionFalse, mdl.GetCondition(xpv1.TypeSynced).Status)

		err = reconcileFn()
		require.NoError(t, err)
		mdl = getModel()
		if diff := cmp.Diff(mdl.GetCondition(xpv1.TypeSynced), xpv1.ReconcileSuccess(), testutils.IgnoreXPv1ConditionFields()); diff != "" {
			t.Fatalf("conditions differ, -got +want:\n%s", diff)
		}
		if diff := cmp.Diff(mdl.GetCondition(xpv1.TypeReady), xpv1.Available(), testutils.IgnoreXPv1ConditionFields()); diff != "" {
			t.Fatalf("conditions differ, -got +want:\n%s", diff)
		}
	})
}
