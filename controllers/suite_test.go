package controllers

import (
	"flag"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"

	pscv1alpha1 "cronprimer.local/api/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var k8sManager ctrl.Manager
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	junitReporter := reporters.NewJUnitReporter(fmt.Sprintf("../TEST-ginkgo-junit_%d.xml", config.GinkgoConfig.ParallelNode))

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{envtest.NewlineReporter{}, junitReporter})
}

var _ = BeforeSuite(func(done Done) {
	logf.SetLogger(zap.LoggerTo(GinkgoWriter, true))

	useCluster := true

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		UseExistingCluster:       &useCluster,
		AttachControlPlaneOutput: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	err = pscv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	// make the metrics listen address different for each parallel thread to avoid clashes when running with -p
	var metricsAddr string
	metricsPort := 8090 + config.GinkgoConfig.ParallelNode
	flag.StringVar(&metricsAddr, "metrics-addr", fmt.Sprintf(":%d", metricsPort), "The address the metric endpoint binds to.")
	flag.Parse()

	k8sManager, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		MetricsBindAddress: metricsAddr,
	})
	Expect(err).ToNot(HaveOccurred())

	// Uncomment the block below to run the operator locally and enable breakpoints / debug during tests
	/*
		err = (&PreScaledCronJobReconciler{
			Client:             k8sManager.GetClient(),
			Log:                ctrl.Log.WithName("controllers").WithName("PrescaledCronJob"),
			Recorder:           k8sManager.GetEventRecorderFor("prescaledcronjob-controller"),
			InitContainerImage: "initcontainer:1",
		}).SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())
	*/

	go func() {
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred())
	}()

	k8sClient = k8sManager.GetClient()
	Expect(k8sClient).ToNot(BeNil())

	close(done)
}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).ToNot(HaveOccurred())
})
