package model

import (
	"context"
	"fmt"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/hashicorp/golang-lru/v2/expirable"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	crosslamav1alpha1 "aerf.io/ollama-operator/apis/crossplane_ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/controllers/crossplane/generic"
)

var (
	errNotModel = "managed resource is not a Model custom resource"
)

// Setup adds a controller that reconciles MyType managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(crosslamav1alpha1.ModelKind)
	l := o.Logger.WithValues("controller", name)
	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}
	opts := []managed.ReconcilerOption{
		managed.WithExternalConnecter(
			&connector{
				kube:   mgr.GetClient(),
				usage:  resource.NewProviderConfigUsageTracker(mgr.GetClient(), &crosslamav1alpha1.ProviderConfigUsage{}),
				logger: l,
				clientCache: expirable.NewLRU(100, func(key string, val client.Client) {
					l.Debug("evicting client from LRU cache", "key", key)
				}, 15*time.Minute),
			}),
		managed.WithLogger(l),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...),
		//managed.WithManagementPolicies(), // TODO
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(crosslamav1alpha1.ModelGroupVersionKind), opts...)
	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&crosslamav1alpha1.Model{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// A connector is expected to produce an ExternalClient when its Connect method
// is called.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	logger      logging.Logger
	clientCache *expirable.LRU[string, client.Client]
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*crosslamav1alpha1.Model)
	if !ok {
		return nil, fmt.Errorf("managed resource is not a %s", crosslamav1alpha1.ModelKind)
	}
	l := c.logger.WithValues("request", cr.Name)

	l.Debug("Connecting")

	pc := &crosslamav1alpha1.ProviderConfig{}

	if cr.GetProviderConfigReference() == nil {
		return nil, errors.New("providerConfigRef is not set")
	}

	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, "failed to track usages of referenced ProviderConfig")
	}

	n := types.NamespacedName{Name: cr.GetProviderConfigReference().Name}
	if err := c.kube.Get(ctx, n, pc); err != nil {
		return nil, errors.Wrap(err, "failed to get provider config")
	}

	var externalK8sCli client.Client
	// no, we cannot use "github.com/crossplane-contrib/provider-kubernetes/pkg/kube/client" like helm&k8s providers, because they have too old deps :/
	switch pc.Spec.Credentials.Source {
	case xpv1.CredentialsSourceInjectedIdentity:
		externalK8sCli = c.kube
	default:
		cachedCli, ok := c.clientCache.Get(pc.GetName())
		if ok {
			externalK8sCli = cachedCli
		}
		content, err := resource.CommonCredentialExtractor(ctx, pc.Spec.Credentials.Source, c.kube, pc.Spec.Credentials.CommonCredentialSelectors)
		if err != nil {
			return nil, fmt.Errorf("failed to extract credentials: %s", err)
		}
		restCfg, err := clientcmd.RESTConfigFromKubeConfig(content)
		if err != nil {
			return nil, fmt.Errorf("failed to build rest config: %s", err)
		}
		restCfg.Burst = 300
		restCfg.QPS = 100
		externalK8sCli, err = client.New(restCfg, client.Options{})
		if err != nil {
			return nil, err
		}
	}
	return generic.NewExternalForType[*crosslamav1alpha1.Model](&external{
		externalK8sCli: externalK8sCli,
		logger:         l,
	}, errors.New(errNotModel)), nil
}

type external struct {
	externalK8sCli client.Client
	logger         logging.Logger
}

func (e *external) Observe(ctx context.Context, model *crosslamav1alpha1.Model) (managed.ExternalObservation, error) {
	e.logger.Debug("Observing", "resource", model)

	current := model.DeepCopy()
	err := e.externalK8sCli.Get(ctx, types.NamespacedName{
		Namespace: current.GetNamespace(),
		Name:      current.GetName(),
	}, current)

	if apierrors.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "failed to get Model")
	}
}

func (e *external) Create(ctx context.Context, model *crosslamav1alpha1.Model) (managed.ExternalCreation, error) {
	// nah I'll end up recreating provider-k8s logic, boring
	//TODO implement me
	panic("implement me")
}

func (e *external) Update(ctx context.Context, model *crosslamav1alpha1.Model) (managed.ExternalUpdate, error) {
	// nah I'll end up recreating provider-k8s logic, boring
	//TODO implement me
	panic("implement me")
}

func (e *external) Delete(_ context.Context, model *crosslamav1alpha1.Model) (managed.ExternalDelete, error) {
	e.logger.Debug("Deleting", "resource", model)
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(ctx context.Context) error {
	e.logger.Debug("Disconnecting")
	return nil
}
