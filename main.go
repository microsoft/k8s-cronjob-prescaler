package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	pscv1alpha1 "cronprimer.local/api/v1alpha1"
	"cronprimer.local/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

const (
	initContainerEnvVariable = "INIT_CONTAINER_IMAGE"
	defaultContainerImage    = "initcontainer:1"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
	probeLog = ctrl.Log.WithName("probe")
)

func init() {
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add client go scheme")
		os.Exit(1)
	}

	if err := pscv1alpha1.AddToScheme(scheme); err != nil {
		setupLog.Error(err, "unable to add pscv1alpha1 scheme")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	logger := zap.Logger(true)
	ctrl.SetLogger(logger)

	setupProbes()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	initContainerImage := os.Getenv(initContainerEnvVariable)

	if initContainerImage == "" {
		setupLog.Info(fmt.Sprintf("%s not set, using default", initContainerEnvVariable))
		initContainerImage = defaultContainerImage
	}

	setupLog.Info(fmt.Sprintf("Using image %s for initContainer", initContainerImage))

	if err = (&controllers.PreScaledCronJobReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("prescaledcronjob"),
		Recorder:           mgr.GetEventRecorderFor("prescaledcronjob-controller"),
		InitContainerImage: initContainerImage,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "prescaledcronjob")
		os.Exit(1)
	}

	if err = (&controllers.PodReconciler{
		Client:             mgr.GetClient(),
		Log:                ctrl.Log.WithName("controllers").WithName("pod"),
		Recorder:           mgr.GetEventRecorderFor("pod-controller"),
		InitContainerImage: initContainerImage,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "pod")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupProbes() {
	setupLog.Info("setting up probes")
	started := time.Now()

	http.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		data := (time.Since(started)).String()
		_, err := w.Write([]byte(data))
		if err != nil {
			probeLog.Error(err, "problem in readiness probe")
			return
		}
	})

	http.HandleFunc("/alive", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, err := w.Write([]byte("ok"))
		if err != nil {
			probeLog.Error(err, "problem in liveness probe")
			return
		}
	})

	go func() {
		setupLog.Info("probes are starting to listen", "addr", ":8081")
		err := http.ListenAndServe(":8081", nil)
		if err != nil {
			setupLog.Error(err, "problem setting up probes")
		}
	}()
}
