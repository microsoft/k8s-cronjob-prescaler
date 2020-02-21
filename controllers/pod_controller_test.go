package controllers

import (
	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/ReneKroon/ttlcache"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Pod Controller", func() {

	Context("With a set of preexisting events", func() {
		// Create a sample pod with last tracked event set
		podName := "bananas"
		lastHighWaterMarkUID := "lasthighwatermark"

		trackedEventsByPod = ttlcache.NewCache()
		trackedEventsByPod.SetWithTTL(podName, types.UID(lastHighWaterMarkUID), time.Hour)

		newEvent := corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				UID: types.UID("new3"),
			},
		}
		oldEvent := corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				UID: types.UID("old2"),
			},
		}

		// Create a list of sorted events with new and old events either side of the high water mark
		sortedEvents := []corev1.Event{
			newEvent,
			{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID(lastHighWaterMarkUID),
				},
			},
			oldEvent,
			{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("old1"),
				},
			},
		}

		It("ttl cache should expire and remove items", func() {
			keyName := "bananas1234"
			trackedEventsByPod.SetWithTTL(keyName, true, time.Second*2)

			// Wait for item to expire
			<-time.After(time.Second * 3)

			_, valueExists := trackedEventsByPod.Get(keyName)
			Expect(valueExists).To(BeFalse())
		})

		It("ttl cache should renew ttl time when items read", func() {
			keyName := "bananas456"
			trackedEventsByPod.SetWithTTL(keyName, true, time.Second*5)

			<-time.After(time.Second * 2)

			// Read before expiry
			_, valueExists := trackedEventsByPod.Get(keyName)
			Expect(valueExists).To(BeTrue())

			<-time.After(time.Second * 4)

			// Read before expiry
			_, valueExists = trackedEventsByPod.Get(keyName)
			Expect(valueExists).To(BeTrue())

			<-time.After(time.Second * 4)

			// Read before expiry
			_, valueExists = trackedEventsByPod.Get(keyName)
			Expect(valueExists).To(BeTrue())

			// Let it expire
			<-time.After(time.Second * 6)

			_, valueExists = trackedEventsByPod.Get(keyName)
			Expect(valueExists).To(BeFalse())
		})

		It("getNewEventsSinceLastRun should only return new events", func() {
			newEvents := getNewEventsSinceLastRun(podName, sortedEvents)

			_, expectMissing := newEvents["old1"]
			Expect(expectMissing).To(BeFalse())

			_, expectMissingHighWater := newEvents["lasthighwatermark"]
			Expect(expectMissingHighWater).To(BeFalse())

			_, expectContains := newEvents["new3"]
			Expect(expectContains).To(BeTrue())
		})

		It("haveNewEventsOccurred should return true", func() {
			newEvents := getNewEventsSinceLastRun(podName, sortedEvents)
			result := allHaveOccurredWithAtLeastOneNew(newEvents, &newEvent)

			Expect(result).To(BeTrue())
		})

		It("haveNewEventsOccurred should return true at least one new event", func() {
			newEvents := getNewEventsSinceLastRun(podName, sortedEvents)
			result := allHaveOccurredWithAtLeastOneNew(newEvents, &newEvent, &oldEvent)

			Expect(result).To(BeTrue())
		})

		It("haveNewEventsOccurred should return false with old events", func() {
			newEvents := getNewEventsSinceLastRun(podName, sortedEvents)
			result := allHaveOccurredWithAtLeastOneNew(newEvents, &oldEvent, &oldEvent)

			Expect(result).To(BeFalse())
		})

		It("isNewEvent should return true for new event", func() {
			newEvents := getNewEventsSinceLastRun(podName, sortedEvents)
			result := isNewEvent(newEvents, &newEvent)

			Expect(result).To(BeTrue())
		})

		It("isNewEvent should return false for old", func() {
			newEvents := getNewEventsSinceLastRun(podName, sortedEvents)
			result := isNewEvent(newEvents, &oldEvent)

			Expect(result).To(BeFalse())
		})
	})

	Context("With a set of kubelet and scheduler events", func() {
		scheduledSampleEvent := mustReadEventFromFile("../testdata/events/scheduledEvent.json")
		initStartedSampleEvent := mustReadEventFromFile("../testdata/events/initStartedEvent.json")
		workloadPullEvent := mustReadEventFromFile("../testdata/events/workloadPulledEvent.json")
		workloadStartedEvent := mustReadEventFromFile("../testdata/events/workloadStartedEvent.json")
		unintestingEvent := mustReadEventFromFile("../testdata/events/uninterestingEvent.json")
		cronSchedule := "*/1 * * * *"

		It("Should correctly identify initStartedEvent", func() {
			isInteresting, eventType := getEventType(initStartedSampleEvent)

			Expect(isInteresting).To(BeTrue())
			Expect(eventType).To(Equal(startedInitContainerEvent))
		})

		It("Should correctly identify scheduledEvent", func() {
			isInteresting, eventType := getEventType(scheduledSampleEvent)

			Expect(isInteresting).To(BeTrue())
			Expect(eventType).To(Equal(scheduledEvent))
		})

		It("Should correctly identify workloadPullEvent", func() {
			isInteresting, eventType := getEventType(workloadPullEvent)

			Expect(isInteresting).To(BeTrue())
			Expect(eventType).To(Equal(finishedInitContainerEvent))
		})

		It("Should correctly identify workloadStartedEvent", func() {
			isInteresting, eventType := getEventType(workloadStartedEvent)

			Expect(isInteresting).To(BeTrue())
			Expect(eventType).To(Equal(startedWorkloadContainerEvent))
		})

		It("Should correctly identify unintestingEvent", func() {
			isInteresting, eventType := getEventType(unintestingEvent)

			Expect(isInteresting).To(BeFalse())
			Expect(eventType).To(Equal(""))
		})

		Context("testing generateTransitionTimingsFromEvents with events", func() {
			allEvents := []corev1.Event{
				scheduledSampleEvent, initStartedSampleEvent, workloadPullEvent, workloadStartedEvent,
			}
			newEvents := map[types.UID]corev1.Event{
				scheduledSampleEvent.UID:   scheduledSampleEvent,
				initStartedSampleEvent.UID: initStartedSampleEvent,
				workloadPullEvent.UID:      workloadPullEvent,
				workloadStartedEvent.UID:   workloadStartedEvent,
			}

			time, err := time.Parse(time.RFC3339, "2020-01-29T12:03:05Z")
			if err != nil {
				panic(err)
			}
			creationTime := metav1.NewTime(time)
			timings, err := generateTransitionTimingsFromEvents(allEvents, newEvents, creationTime, cronSchedule)

			It("Shouldn't error", func() {
				Expect(err).To(BeNil())
			})

			It("Should get correct dates from events", func() {
				Expect(timings.createdAt).NotTo(Equal(nil))
				Expect(timings.scheduledAt).NotTo(Equal(nil))
				Expect(timings.initStartAt).NotTo(Equal(nil))
				Expect(timings.initFinishedAt).NotTo(Equal(nil))
				Expect(timings.workloadStartAt).NotTo(Equal(nil))

				outputFormat := "2006-01-02T15:04:05Z"                                                               // Format the date into format used in event.json files
				Expect(timings.scheduledAt.LastTimestamp.Format(outputFormat)).To(Equal("2020-01-29T12:03:07Z"))     // Time from scheduledEvent.json
				Expect(timings.initStartAt.LastTimestamp.Format(outputFormat)).To(Equal("2020-01-29T12:03:08Z"))     // Time from initStartedEvent.json
				Expect(timings.initFinishedAt.LastTimestamp.Format(outputFormat)).To(Equal("2020-01-29T12:04:03Z"))  // Time from workloadPulledEvent.json
				Expect(timings.workloadStartAt.LastTimestamp.Format(outputFormat)).To(Equal("2020-01-29T12:04:05Z")) // Time from workloadStartedEvent.json
			})

			It("Should get correct transition times", func() {
				Expect(timings.transitionsObserved[timeToSchedule].String()).To(Equal("2s"))        // Time between Create and Schedule
				Expect(timings.transitionsObserved[timeInitContainerRan].String()).To(Equal("55s")) // Time between initStartEvent.json and workloadPullEvent.json
				Expect(timings.transitionsObserved[timeToStartWorkload].String()).To(Equal("2s"))   // Time between workloadPullEvent.json and worloadStartEvent.json
				Expect(timings.transitionsObserved[timeDelayOfWorkload].String()).To(Equal("5s"))   // Based on the cron schedule how late was the workload
			})

			It("Should only calculate new transition times", func() {
				reducedNewEvents := map[types.UID]corev1.Event{
					workloadPullEvent.UID:    workloadPullEvent,
					workloadStartedEvent.UID: workloadStartedEvent,
				}
				expectedReducedTimings, err := generateTransitionTimingsFromEvents(allEvents, reducedNewEvents, creationTime, cronSchedule)
				Expect(err).To(BeNil())

				_, timeToScheduleExists := expectedReducedTimings.transitionsObserved[timeToSchedule]
				Expect(timeToScheduleExists).To(BeFalse())
				Expect(expectedReducedTimings.transitionsObserved[timeInitContainerRan].String()).To(Equal("55s"))
				Expect(expectedReducedTimings.transitionsObserved[timeToStartWorkload].String()).To(Equal("2s"))
			})
		})

	})
})

func mustReadEventFromFile(filepath string) corev1.Event {
	dat, err := ioutil.ReadFile(filepath)
	if err != nil {
		panic(err)
	}
	event := &corev1.Event{}
	if err := json.Unmarshal(dat, event); err != nil {
		panic(err)
	}
	return *event
}
