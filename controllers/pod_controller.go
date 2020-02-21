package controllers

import (
	"context"
	"fmt"
	"sort"
	"time"

	pscv1alpha1 "cronprimer.local/api/v1alpha1"
	"github.com/ReneKroon/ttlcache"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/robfig/cron/v3"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var trackedEventsByPod = ttlcache.NewCache()

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	clientset          *kubernetes.Clientset
	Log                logr.Logger
	Recorder           record.EventRecorder
	InitContainerImage string
}

const (
	timeToSchedule       = "timeToSchedule"
	timeInitContainerRan = "timeInitContainerRan"
	timeToStartWorkload  = "timeToStartWorkload"
	timeDelayOfWorkload  = "timeDelayOfWorkload"

	scheduledEvent                = "Scheduled"
	startedInitContainerEvent     = "StartedInitContainer"
	finishedInitContainerEvent    = "FinishedInitContainer"
	startedWorkloadContainerEvent = "StartedWorkloadContainer"
)

type podTransitionTimes struct {
	createdAt       *metav1.Time
	scheduledAt     *corev1.Event
	initStartAt     *corev1.Event
	initFinishedAt  *corev1.Event
	workloadStartAt *corev1.Event

	transitionsObserved map[string]time.Duration
}

// +kubebuilder:rbac:groups="",resources=events,verbs=get;list;patch;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get

// Reconcile watches for Pods created as a results of a PrimedCronJob and tracks metrics against the parent
// PrimedCronJob about the instance by inspecting the events on the pod (for example: late, early, init container runtime)
func (r *PodReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("pod", req.NamespacedName)

	logger.Info(fmt.Sprintf("Starting reconcile loop for %v", req.NamespacedName))
	defer logger.Info(fmt.Sprintf("Finish reconcile loop for %v", req.NamespacedName))

	podInstance := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, podInstance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get pod")
		return ctrl.Result{}, err
	}

	parentExists, prescaledInstance, err := r.getParentPrescaledCronIfExists(ctx, podInstance)
	if err != nil {
		logger.Error(err, "Failed to get parent prescaledcronjob")
		return ctrl.Result{}, err
	}
	if !parentExists {
		logger.Info("prescaledcronjob no longer exists, likely deleted recently")
		return ctrl.Result{}, nil
	}

	// Lets build some stats
	eventsOnPodOverLastHour, err := r.clientset.CoreV1().Events(podInstance.Namespace).List(metav1.ListOptions{
		FieldSelector: fields.AndSelectors(fields.OneTermEqualSelector("involvedObject.name", podInstance.Name), fields.OneTermEqualSelector("involvedObject.namespace", podInstance.Namespace)).String(),
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	// We don't care about this one it has no events yet
	if len(eventsOnPodOverLastHour.Items) < 1 {
		return ctrl.Result{}, nil
	}

	// When we do have some events
	// Lets make sure we have time in time order
	// latest -> oldest
	allEvents := eventsOnPodOverLastHour.Items
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].FirstTimestamp.After(allEvents[j].FirstTimestamp.Time)
	})

	newEventsSinceLastRun := getNewEventsSinceLastRun(podInstance.Name, allEvents)

	// Update last tracked event
	// Track with a 75min TTL to ensure list doesn't grow forever (events exist for 1 hour by default in k8s added a buffer)
	trackedEventsByPod.SetWithTTL(podInstance.Name, allEvents[0].UID, time.Minute*75)

	// No new events - give up
	if len(newEventsSinceLastRun) < 1 {
		return ctrl.Result{}, nil
	}

	// Calculate the timings of transitions between states
	timings, err := generateTransitionTimingsFromEvents(allEvents, newEventsSinceLastRun, podInstance.CreationTimestamp, prescaledInstance.Spec.CronJob.Spec.Schedule)
	if err != nil {
		//generateTransitionTimings errors are only partial faults so can log and continue
		// worst case this error means a transition time wasn't available
		r.Recorder.Event(prescaledInstance, corev1.EventTypeWarning, "Metrics", err.Error())
	}

	r.publishMetrics(timings, podInstance, prescaledInstance)

	r.Recorder.Event(prescaledInstance, corev1.EventTypeNormal, "Debug", "Metrics calculated for PrescaleCronJob invocation.")

	return ctrl.Result{}, nil
}

func (r *PodReconciler) getParentPrescaledCronIfExists(ctx context.Context, podInstance *corev1.Pod) (exists bool, instance *pscv1alpha1.PreScaledCronJob, err error) {
	// Attempt to get the parent name from the pod
	prescaledName, exists := podInstance.GetLabels()[primedCronLabel]
	if !exists {
		return false, nil, nil
	}

	// Get the prescaled cron which triggered this pod
	prescaledInstance := &pscv1alpha1.PreScaledCronJob{}
	if err := r.Get(ctx, types.NamespacedName{Name: prescaledName, Namespace: podInstance.Namespace}, prescaledInstance); err != nil {
		if errors.IsNotFound(err) {
			return false, nil, nil
		}
		return true, nil, err
	}
	return true, prescaledInstance, nil
}

func getNewEventsSinceLastRun(podName string, latestEventsFirst []corev1.Event) map[types.UID]corev1.Event {
	// Work out which events we've already processed and filter to new events only
	eventsSinceLastCheck := map[types.UID]corev1.Event{}
	uidOfLastProcessedEvent, isCurrentlyTrackedPod := trackedEventsByPod.Get(podName)

	for _, event := range latestEventsFirst {
		if isCurrentlyTrackedPod && event.UID == uidOfLastProcessedEvent {
			// We've caught up to last seen event
			break
		}
		eventsSinceLastCheck[event.UID] = event
	}

	return eventsSinceLastCheck
}

func generateTransitionTimingsFromEvents(allEvents []corev1.Event, newEventsSinceLastRun map[types.UID]corev1.Event, podCreationTime metav1.Time, cronSchedule string) (podTransitionTimes, error) {
	// What do we know?
	timings := podTransitionTimes{
		createdAt:           &podCreationTime,
		transitionsObserved: map[string]time.Duration{},
	}

	// Build idea of timing of events
	for _, event := range allEvents {
		eventCaptured := event
		interested, eventType := getEventType(eventCaptured)
		if !interested {
			continue
		}

		switch eventType {
		case scheduledEvent:
			timings.scheduledAt = &eventCaptured
		case startedInitContainerEvent:
			timings.initStartAt = &eventCaptured
		case finishedInitContainerEvent:
			timings.initFinishedAt = &eventCaptured
		case startedWorkloadContainerEvent:
			timings.workloadStartAt = &eventCaptured
		}
	}

	// Calculate transition durations which are based off new events
	if timings.scheduledAt != nil && isNewEvent(newEventsSinceLastRun, timings.scheduledAt) {
		timings.transitionsObserved[timeToSchedule] = timings.scheduledAt.LastTimestamp.Sub(timings.createdAt.Time)
	}

	if allHaveOccurredWithAtLeastOneNew(newEventsSinceLastRun, timings.initStartAt, timings.initFinishedAt) {
		timings.transitionsObserved[timeInitContainerRan] = timings.initFinishedAt.LastTimestamp.Sub(timings.initStartAt.LastTimestamp.Time)
	}

	if allHaveOccurredWithAtLeastOneNew(newEventsSinceLastRun, timings.initFinishedAt, timings.workloadStartAt) {
		timings.transitionsObserved[timeToStartWorkload] = timings.workloadStartAt.LastTimestamp.Sub(timings.initFinishedAt.LastTimestamp.Time)
	}

	if allHaveOccurredWithAtLeastOneNew(newEventsSinceLastRun, timings.workloadStartAt) {
		// Todo: Track as vectored metric by early/late
		schedule, err := cron.ParseStandard(cronSchedule)
		if err != nil {
			return timings, fmt.Errorf("Parital failure generating transition times, failed to parse CRON Schedule: %s", err.Error())
		}

		expectedStartTimeForWorkload := schedule.Next(podCreationTime.Time)
		timings.transitionsObserved[timeDelayOfWorkload] = timings.workloadStartAt.LastTimestamp.Time.Sub(expectedStartTimeForWorkload)
	}

	return timings, nil
}

func (r *PodReconciler) publishMetrics(timings podTransitionTimes, pod *corev1.Pod, prescaledInstance *pscv1alpha1.PreScaledCronJob) {
	agentpool, exists := pod.Spec.NodeSelector["agentpool"]
	if !exists {
		agentpool = "noneset"
	}

	for transitionName, duration := range timings.transitionsObserved {
		r.Recorder.Eventf(prescaledInstance, corev1.EventTypeNormal, "Metrics", "Event %s took %s on pod %s", transitionName, duration.String(), pod.Name)

		durationSecs := duration.Seconds()

		durationType := "late"
		if durationSecs < 0 {
			durationType = "early"
			durationSecs = durationSecs * -1
		}
		promLabels := prometheus.Labels{"prescalecron": prescaledInstance.Name, "nodepool": agentpool, "durationtype": durationType}

		histogram, exists := transitionTimeHistograms[transitionName]
		if !exists {
			r.Recorder.Eventf(prescaledInstance, corev1.EventTypeWarning, "Metrics", "Failed to track transition time as no histogram defined for %s", transitionName)
		}
		histogram.With(promLabels).Observe(durationSecs)
	}
}

func allHaveOccurredWithAtLeastOneNew(newEvents map[types.UID]corev1.Event, events ...*corev1.Event) bool {
	atLeastOneNewEvent := false
	for _, event := range events {
		// If event is nil it hasn't happened yet
		if event == nil {
			return false
		}

		// Check at least one of the events is new since last time
		// the reconcile loop ran.
		if !atLeastOneNewEvent {
			atLeastOneNewEvent = isNewEvent(newEvents, event)
		}
	}
	return atLeastOneNewEvent
}

func isNewEvent(newEvents map[types.UID]corev1.Event, thisEvent *corev1.Event) bool {
	_, existsInDictorary := newEvents[thisEvent.UID]

	return existsInDictorary
}

func getEventType(event corev1.Event) (isInteresting bool, eventType string) {
	// If this a sheduler assigned event?
	if event.Reason == "Scheduled" {
		return true, scheduledEvent
	}

	// Kubelet events
	if event.Source.Component == "kubelet" {
		// Any other field spec if related to original workload
		isRelatedToInitContainer := event.InvolvedObject.FieldPath == fmt.Sprintf("spec.initContainers{%s}", warmupContainerInjectNameUID)

		// Are these events related to our init container?
		if isRelatedToInitContainer {
			if event.Reason == "Started" {
				return true, startedInitContainerEvent
			}
		}

		// When the init container is finished the other containers in the pod will get pulled
		// if the image is already on the node "Pulled" is fired if not "Pulling" -> "Pulled"
		// This is useful as it signals when our init container has exited and now kubelet
		// has moved to running the original workload
		if (event.Reason == "Pulling" || event.Reason == "Pulled") && !isRelatedToInitContainer {
			return true, finishedInitContainerEvent
		}

		// Main workload has started (or at least one of them)
		if event.Reason == "Started" && !isRelatedToInitContainer {
			return true, startedWorkloadContainerEvent
		}
	}

	return false, ""
}

// SetupWithManager sets up defaults
func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Get clientset so we can read events
	r.clientset = kubernetes.NewForConfigOrDie(mgr.GetConfig())

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.Funcs{
			// We're using the pod controller to watch for the job moving from init -> normal execution
			// given this we don't care about Delete or Create only update and only update on
			// pods which are downstream of the CronPrimer object
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			CreateFunc: func(e event.CreateEvent) bool {
				return true
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Going for an injected pod label instead
				if _, exists := e.MetaNew.GetLabels()[primedCronLabel]; exists {
					return true
				}
				return false
			},
		}).
		Complete(r)
}
