package controllers

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	successMetric = "success"
	failureMetric = "failure"
	// CronJobCreatedMetric represents a metric to track cronjob created
	CronJobCreatedMetric = "create"
	// CronJobUpdatedMetric represents a metric to track cronjob updated
	CronJobUpdatedMetric = "update"
	// CronJobDeletedMetric represents a metric to track cronjob deleted
	CronJobDeletedMetric = "delete"
)

var cronjobCounter = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "prescaledcronjoboperator_cronjob_action_total",
	Help: "Number of CronJob objects created by controller",
}, []string{"action", "outcome"})

var timingLabels = []string{"prescalecron", "nodepool", "durationtype"}

// Track in buckets from 2 secs up to 60mins over 28 increments
var timingBuckets = prometheus.ExponentialBuckets(2, 1.32, 28)
var transitionTimeHistograms = map[string]*prometheus.HistogramVec{
	timeToSchedule:       timeToScheduleHistogram,
	timeInitContainerRan: timeInitContainerRanHistogram,
	timeToStartWorkload:  timeToStartWorkloadHistogram,
	timeDelayOfWorkload:  timeDelayOfWorkloadHistogram,
}

var timeToScheduleHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "prescalecronjoboperator_cronjob_time_to_schedule",
	Help:    "How long did it take to schedule the pod used to execute and instance of the CRONJob in secs",
	Buckets: timingBuckets,
}, timingLabels)

var timeInitContainerRanHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "prescalecronjoboperator_cronjob_time_init_container_ran",
	Help:    "How long did the warmup container run waiting for the cron schedule to trigger in secs",
	Buckets: timingBuckets,
}, timingLabels)

var timeToStartWorkloadHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "prescalecronjoboperator_cronjob_time_to_start_workload",
	Help:    "How long did it take to start the real workload after warmup container stopped in secs",
	Buckets: timingBuckets,
}, timingLabels)

var timeDelayOfWorkloadHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "prescalecronjoboperator_cronjob_time_delay_of_workload",
	Help:    "How long did after it's scheduled start time did the workload actually start in secs",
	Buckets: timingBuckets,
}, timingLabels)

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(cronjobCounter)
	metrics.Registry.MustRegister(timeToScheduleHistogram)
	metrics.Registry.MustRegister(timeInitContainerRanHistogram)
	metrics.Registry.MustRegister(timeToStartWorkloadHistogram)
	metrics.Registry.MustRegister(timeDelayOfWorkloadHistogram)
}

// TrackCronAction increments the metric tracking how many CronJobs actions
func TrackCronAction(action string, success bool) {

	outcome := successMetric

	if !success {
		outcome = failureMetric
	}

	cronjobCounter.WithLabelValues(action, outcome).Inc()
}
