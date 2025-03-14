/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"net/http/pprof"
	"os"
	stdruntime "runtime"
	"strconv"
	"time"

	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/felixge/fgprof"
	"github.com/hashicorp/go-cleanhttp"
	"github.com/spf13/pflag"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	otelsdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/atomic"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/server/routes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/tracing"
	tracingapi "k8s.io/component-base/tracing/api/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	ollamav1alpha1 "aerf.io/ollama-operator/apis/ollama/v1alpha1"
	"aerf.io/ollama-operator/internal/commonmeta"
	"aerf.io/ollama-operator/internal/controllers/model"
	"aerf.io/ollama-operator/internal/controllers/prompt"
	"aerf.io/ollama-operator/internal/restconfig"

	"aerf.io/k8sutils/k8stracing"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	_ "k8s.io/component-base/logs/json/register"
)

func adjustedLogOptions() *logsapi.LoggingConfiguration {
	opts := logs.NewOptions()
	opts.Format = logsapi.JSONLogFormat
	opts.Verbosity = logsapi.VerbosityLevel(0)
	return opts
}

var (
	scheme                              = runtime.NewScheme()
	setupLog                            = ctrl.Log.WithName("setup")
	logOptions                          = adjustedLogOptions()
	printHelpAndExit                    = false
	diagnosticsPort                     = 8080
	probePort                           = 8081
	profilerPort                        = 8082
	enableLeaderElection                = false
	leaderElectionLeaseDuration         = 15 * time.Second
	leaderElectionRenewDeadline         = 10 * time.Second
	leaderElectionRetryPeriod           = 2 * time.Second
	watchNamespace                      = metav1.NamespaceAll
	restConfigQPS               float32 = 100
	restConfigBurst                     = 300
	dstGroupKindConcurrency             = map[string]int{
		ollamav1alpha1.ModelGroupVersionKind.GroupKind().String():  10,
		ollamav1alpha1.PromptGroupVersionKind.GroupKind().String(): 10,
	}
	groupKindConcurrency               = maps.Clone(dstGroupKindConcurrency)
	tracingEndpoint                    = ""
	tracingSampingRatePerMillion int32 = 0

	blockProfileRate     = 0
	cpuProfileRate       = 0
	mutexProfileFraction = 0
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(ollamav1alpha1.AddToScheme(scheme))
}

func initFlags(fs *pflag.FlagSet) {
	logsapi.AddFlags(logOptions, fs)
	// hide alpha lvl flags
	// and the ones we do not recommend to use
	for _, flagName := range []string{
		"vmodule",
		"log-json-info-buffer-size",
		"log-json-split-stream",
		"log-text-info-buffer-size",
		"log-text-split-stream",
	} {
		if err := fs.MarkHidden(flagName); err != nil {
			panic(err)
		}
	}

	fs.BoolVar(&enableLeaderElection, "leader-elect", enableLeaderElection,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	fs.DurationVar(&leaderElectionLeaseDuration, "leader-elect-lease-duration", leaderElectionLeaseDuration,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")

	fs.DurationVar(&leaderElectionRenewDeadline, "leader-elect-renew-deadline", leaderElectionRenewDeadline,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")

	fs.DurationVar(&leaderElectionRetryPeriod, "leader-elect-retry-period", leaderElectionRetryPeriod,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")

	fs.StringVar(&watchNamespace, "namespace", watchNamespace,
		"Namespace that the operator watches to reconcile ollama.aerf.io objects. If unspecified, the operator watches objects across all namespaces.")

	fs.IntVar(&profilerPort, "profiler-port", profilerPort,
		"Port to expose the pprof profiler")

	fs.IntVar(&diagnosticsPort, "diagnostics-port", diagnosticsPort,
		"Port to expose diagnostics endpoint")

	fs.IntVar(&probePort, "health-probe-port", probePort,
		"Port of the probe endpoint")

	fs.Float32Var(&restConfigQPS, "kube-api-qps", restConfigQPS,
		"Maximum queries per second from the controller client to the Kubernetes API server.")

	fs.IntVar(&restConfigBurst, "kube-api-burst", restConfigBurst,
		"Maximum number of queries that should be allowed in one burst from the controller client to the Kubernetes API server.")

	fs.BoolVarP(&printHelpAndExit, "help", "h", printHelpAndExit, "Prints flag documentation and exits")

	fs.StringToIntVar(&groupKindConcurrency, "group-kind-concurrency", groupKindConcurrency,
		"A map from a Kind to the number of concurrent reconciliation allowed for that controller. "+
			"The key is expected to be consistent in form with GroupKind.String(), e.g. ReplicaSet in apps group (regardless of version) would be \"ReplicaSet.apps\". "+
			"Anything set by this flag is merged with defaults, so that setting it for e.g Prompts does not remove defaults for Model CRD")

	fs.StringVar(&tracingEndpoint, "tracing-endpoint", tracingEndpoint,
		"Endpoint of the collector this component will report traces to. The connection is insecure, and does not currently support TLS.")

	fs.Int32Var(&tracingSampingRatePerMillion, "tracing-sampling-rate-per-million", tracingSampingRatePerMillion,
		"The number of samples to collect per million spans. Ignored if --tracing-endpoint is not set")

	fs.IntVar(&blockProfileRate, "block-profile-rate", blockProfileRate,
		"Value to provide to runtime.SetBlockProfileRate. Not used if set to 0")

	fs.IntVar(&cpuProfileRate, "cpu-profile-rate", cpuProfileRate,
		"Value to provide to runtime.SetCPUProfileRate. Not used if set to 0")

	fs.IntVar(&mutexProfileFraction, "mutex-profile-fraction", mutexProfileFraction,
		"Value to provide to runtime.SetMutexProfileFraction. Not used if set to 0")
}

func main() {
	if err := mainErr(); err != nil {
		klog.Background().Error(err, "failed to run the application")
		os.Exit(1)
	}
}

func mainErr() error {
	initFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	maps.Copy(dstGroupKindConcurrency, groupKindConcurrency) // this allows us to set concurrency for only 1 CRD without overwriting defaults from other CRDs in this map

	if printHelpAndExit {
		pflag.Usage()
		return nil
	}

	if err := logsapi.ValidateAndApply(logOptions, nil); err != nil {
		return fmt.Errorf("failed to validate logs: %s", err)
	}
	// klog.Background will automatically use the right logger.
	log := klog.Background()
	ctrl.SetLogger(klog.Background())

	var tracingConfig *tracingapi.TracingConfiguration
	if tracingEndpoint != "" {
		tracingConfig = &tracingapi.TracingConfiguration{
			Endpoint:               ptr.To(tracingEndpoint),
			SamplingRatePerMillion: ptr.To(tracingSampingRatePerMillion),
		}
	} else if tracingEndpoint == "" && tracingSampingRatePerMillion != 0 {
		klog.Warningf("--tracing-endpoint was not set, but other tracing configuration flags were set, tracing will remain disabled")
	}

	if err := tracingapi.ValidateTracingConfiguration(tracingConfig, nil, field.NewPath("tracing")).ToAggregate(); err != nil {
		return fmt.Errorf("failed to validate tracing configuration: %s", err)
	}
	log.Info("tracing configuration", "config", tracingConfig)

	ctx := ctrl.SetupSignalHandler()
	otel.SetTextMapPropagator(tracing.Propagators())
	otel.SetMeterProvider(metricnoop.NewMeterProvider()) // # https://github.com/open-telemetry/opentelemetry-go-contrib/issues/5190
	tp, err := tracing.NewProvider(ctx, tracingConfig, nil, []otelsdkresource.Option{
		otelsdkresource.WithAttributes(
			semconv.ServiceNameKey.String("ollama-operator"),
		),
	})
	if err != nil {
		return fmt.Errorf("failed to create tracing provider: %s", err)
	}
	/*
		if secureMetrics {
			// FilterProvider is used to protect the metrics endpoint with authn/authz.
			// These configurations ensure that only authorized users and service accounts
			// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
			// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/metrics/filters#WithAuthenticationAndAuthorization
			metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
		}
	*/

	restCfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config for connecting to k8s apiserver: %s", err)
	}
	restconfig.Adjust(
		restCfg,
		restConfigQPS,
		restConfigBurst,
		"ollama-operator")

	containsImageSelector := labels.SelectorFromSet(map[string]string{"ollama.aerf.io/contains-image": "true"})
	cacheOpts := cache.Options{
		ReaderFailOnMissingInformer: true, // let's try to ensure we understand what resources are cached by disabling auto-cache-creation and doing it manually here
		ByObject: map[client.Object]cache.ByObject{
			&ollamav1alpha1.Model{}:  {},
			&ollamav1alpha1.Prompt{}: {},
			/*
				exposes ollama sts
			*/
			&corev1.Service{}: {
				Label: labels.SelectorFromSet(commonmeta.ManagedByLabel),
			},
			/*
				runs ollama container
			*/
			&appsv1.StatefulSet{}: {
				Label: labels.SelectorFromSet(commonmeta.ManagedByLabel),
			},
			/*
				used to read image data in Prompt controller
			*/
			&corev1.ConfigMap{}: {
				Label: containsImageSelector,
			},
			/*
				used to read image data in Prompt controller
			*/
			&corev1.Secret{}: {
				Label: containsImageSelector,
			},
		},
	}
	if watchNamespace != "" {
		cacheOpts.DefaultNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	logLvl := atomic.NewString(strconv.Itoa(log.GetV()))
	mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: fmt.Sprintf(":%d", probePort),
		Metrics: metricsserver.Options{
			SecureServing: false,
			BindAddress:   fmt.Sprintf(":%d", diagnosticsPort),
			ExtraHandlers: map[string]http.Handler{
				// Add handler to dynamically change log level
				"PUT /debug/flags/v": routes.StringFlagPutHandler(func(arg string) (string, error) {
					msg, err := logs.GlogSetter(arg)
					if err != nil {
						return "", err
					}
					logLvl.Store(arg)
					return msg, nil
				}),
				"GET /debug/flags/v": http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/plain")
					w.Header().Set("X-Content-Type-Options", "nosniff")
					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, logLvl.Load())
				}),
				// Add pprof handler.
				"GET /debug/pprof/":        http.HandlerFunc(pprof.Index),
				"GET /debug/pprof/cmdline": http.HandlerFunc(pprof.Cmdline),
				"GET /debug/pprof/profile": http.HandlerFunc(pprof.Profile),
				"GET /debug/pprof/symbol":  http.HandlerFunc(pprof.Symbol),
				"GET /debug/pprof/trace":   http.HandlerFunc(pprof.Trace),
				"GET /debug/fgprof":        fgprof.Handler(),
			},
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "ollama-operator.aerf.io",
		LeaseDuration:    &leaderElectionLeaseDuration,
		RenewDeadline:    &leaderElectionRenewDeadline,
		RetryPeriod:      &leaderElectionRetryPeriod,
		NewClient: func(cfg *rest.Config, opts client.Options) (client.Client, error) {
			cli, err := client.New(cfg, opts)
			if err != nil {
				return nil, err
			}
			cli = client.WithFieldValidation(cli, metav1.FieldValidationStrict)
			cli = client.WithFieldOwner(cli, "ollama-operator")
			cli = k8stracing.NewK8sClient(cli, tp)
			return cli, nil
		},
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		LeaderElectionReleaseOnCancel: true, // tracing provider shutdown is in a runnable
		Controller: config.Controller{
			GroupKindConcurrency: groupKindConcurrency,
		},
		PprofBindAddress: fmt.Sprintf(":%d", profilerPort),
		Cache:            cacheOpts,
		Client: client.Options{
			Cache: &client.CacheOptions{
				Unstructured: false, // so that we do not cache unstructureds by default
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %s", err)
	}

	httpCli := &http.Client{
		Transport: otelhttp.NewTransport(
			cleanhttp.DefaultPooledTransport(),
			otelhttp.WithPropagators(tracing.Propagators()),
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithPublicEndpoint(),
			otelhttp.WithMeterProvider(metricnoop.NewMeterProvider()),
		),
	}

	if err := model.SetupWithManager(mgr, httpCli, tp); err != nil {
		return fmt.Errorf("failed to setup Model controller: %s", err)
	}

	if err := prompt.SetupWithManager(mgr, httpCli, tp); err != nil {
		return fmt.Errorf("failed to setup Prompt controller: %s", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add healthz checker: %s", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add readyz checker: %s", err)
	}

	if err := mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		<-ctx.Done()
		// new context, not based on ctx
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second*10)
		defer shutdownCancel()
		log.V(1).Info("shutting down tracing provider")
		return errors.Wrap(tp.Shutdown(shutdownCtx), "failed to shutdown tracing provider")
	})); err != nil {
		return fmt.Errorf("failed to add runnable func: %s", err)
	}

	if blockProfileRate != 0 {
		stdruntime.SetBlockProfileRate(blockProfileRate)
	}
	if cpuProfileRate != 0 {
		stdruntime.SetCPUProfileRate(cpuProfileRate)
	}
	if mutexProfileFraction != 0 {
		old := stdruntime.SetMutexProfileFraction(mutexProfileFraction)
		log.Info("reported mutexProfileFraction", "value", old)
	}

	setupLog.Info("starting manager")
	return errors.Wrap(mgr.Start(ctx), "manager problem running manager")
}
