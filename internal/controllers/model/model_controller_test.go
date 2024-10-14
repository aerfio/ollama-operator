package model

import (
	"context"
	"net/http"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/go-logr/logr/testr"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/ollamaclient"
	"aerf.io/ollama-operator/internal/testutils"
)

// test apiserver does not return GVK inside the struct, what the hell
type addGVKReconciler struct {
	inner reconcile.ObjectReconciler[*ollamav1alpha1.Model]
}

func (a addGVKReconciler) Reconcile(ctx context.Context, object *ollamav1alpha1.Model) (reconcile.Result, error) {
	object.SetGroupVersionKind(ollamav1alpha1.ModelGroupVersionKind)
	return a.inner.Reconcile(ctx, object)
}

func TestReconciler_Reconcile(t *testing.T) {
	require.NoError(t, ollamav1alpha1.Install(scheme.Scheme))
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

		r := reconcile.AsReconciler[*ollamav1alpha1.Model](cli,
			&addGVKReconciler{
				inner: &Reconciler{
					client:               client.WithFieldOwner(cli, "ollama-operator"),
					recorder:             record.NewFakeRecorder(1000),
					baseHTTPClient:       &http.Client{},
					tp:                   noop.NewTracerProvider(),
					ollamaClientProvider: ollamaclient.NewProvider(&http.Client{}, noop.NewTracerProvider().Tracer("tracer")),
				}},
		)
		reconcile := func() error {
			_, err := r.Reconcile(ctrl.LoggerInto(context.Background(), testr.NewWithOptions(t, testr.Options{Verbosity: 10})), reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: model.GetNamespace(),
					Name:      model.GetName(),
				},
			})
			return err
		}

		err = reconcile()
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

		//sts := &appsv1.StatefulSet{}
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
		err = reconcile()
		require.NoError(t, err)

		mdl = getModel()
		t.Logf("status: %#v", mdl.Status)
	})
}
