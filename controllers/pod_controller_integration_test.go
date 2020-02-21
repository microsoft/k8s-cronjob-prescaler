package controllers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/robfig/cron/v3"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

var _ = Describe("LONG TEST: Pod Controller metrics", func() {
	logger := log.New(GinkgoWriter, "INFO: ", log.Lshortfile)

	const timeout = time.Second * 145
	const interval = time.Second * 15

	ctx := context.Background()

	BeforeEach(func() {
		// failed test runs that don't clean up leave resources behind.
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	It("with prescaled cronjob sees metrics events", func() {

		// construct a prescaled cron in code + post to K8s
		toCreateFull := generatePSCSpec()
		toCreate := &toCreateFull
		toCreate.Spec.CronJob.Spec.Schedule = fmt.Sprintf("*/1 * * * *")
		toCreate.Spec.WarmUpTimeMins = 0

		Expect(k8sClient.Create(ctx, toCreate)).To(Succeed(), "Creating prescaled cron primer failed and it shouldn't have")

		// Wait till the first execution has started of the pod
		schedule, err := cron.ParseStandard(toCreate.Spec.CronJob.Spec.Schedule)
		Expect(err).To(BeNil())
		expectedPodStartTime := schedule.Next(time.Now()).Add(15 * time.Second)

		<-time.After(time.Until(expectedPodStartTime))

		// Then mark the primed cron as inactive so we're only testing one instance
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: toCreate.Namespace, Name: toCreate.Name}, toCreate)
		Expect(err).To(BeNil())

		suspend := true
		toCreate.Spec.CronJob.Spec.Suspend = &suspend
		Expect(k8sClient.Update(ctx, toCreate)).To(Succeed(), "Failed to suspend cronjob. We need it suspended so multiple instances of the CRON job don't start - we want to test with just one execution")

		clientset := kubernetes.NewForConfigOrDie(cfg)

		Eventually(func() bool {
			events, err := clientset.CoreV1().Events(namespace).List(metav1.ListOptions{
				FieldSelector: fields.AndSelectors(
					fields.OneTermEqualSelector("involvedObject.name", toCreate.Name),
					fields.OneTermEqualSelector("involvedObject.namespace", toCreate.Namespace),
				).String(),
			})
			logger.Println(len(events.Items))
			if err != nil {
				logger.Println(err.Error())
				Fail("we broke")
				return false
			}

			//For each invocation we generate "Metrics" 4 events. Lets check they all came through.

			// Todo: make this a better test
			// It doesn't quite assert what I want. I want to see that exactly one of each of these timings is reported,
			// what is currently done is roughly equivalent but if timing issues occur then you could pass this test with 4xScheduledAt messages rather than one of each.
			// Will add a task to fix this one up.
			metricsMessageCount := 0
			for _, event := range events.Items {
				if strings.Contains(event.Message, timeToSchedule) ||
					strings.Contains(event.Message, timeInitContainerRan) ||
					strings.Contains(event.Message, timeToStartWorkload) ||
					strings.Contains(event.Message, timeDelayOfWorkload) {

					metricsMessageCount++
				}
			}

			for _, event := range events.Items {
				// We observed an invocation of the job ... Yey!
				if event.Reason == "Debug" && strings.HasPrefix(event.Message, "Metrics calculated for PrescaleCronJob invocation.") {
					logger.Println("Found debug message from controller")

					const expectedNumberOfMetricsEvents = 4
					logger.Printf("Checking number of metrics events reported:%d %+v", metricsMessageCount, events)

					return metricsMessageCount >= expectedNumberOfMetricsEvents
				}
			}

			logger.Println("Event not found on object yet")
			return false
		}, timeout, interval).Should(BeTrue())
	})
})
