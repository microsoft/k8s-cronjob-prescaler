package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pscv1alpha1 "cronprimer.local/api/v1alpha1"
)

// PreScaledCronJobReconciler reconciles a PreScaledCronJob object
type PreScaledCronJobReconciler struct {
	client.Client
	Log                logr.Logger
	Recorder           record.EventRecorder
	InitContainerImage string
}

// +kubebuilder:rbac:groups=psc.cronprimer.local,resources=prescaledcronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=psc.cronprimer.local,resources=prescaledcronjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=cronjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list
// +kubebuilder:rbac:groups="",resources=pods/status,verbs=get

const (
	objectHashField              = "pscObjectHash"
	finalizerName                = "foregroundDeletion"
	primedCronLabel              = "primedcron"
	warmupContainerInjectNameUID = "injected-0d825b4f-07f0-4952-8150-fba894c613b1"
)

// Reconcile takes the PreScaled request and creates a regular cron, n mins earlier.
func (r *PreScaledCronJobReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	logger := r.Log.WithValues("prescaledcronjob", req.NamespacedName)

	logger.Info(fmt.Sprintf("Starting reconcile loop for %v", req.NamespacedName))
	defer logger.Info(fmt.Sprintf("Finish reconcile loop for %v", req.NamespacedName))

	// instance = the submitted prescaledcronjob CRD
	instance := &pscv1alpha1.PreScaledCronJob{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get prescaledcronjob")
		return ctrl.Result{}, err
	}

	// allow cascade delete of child resources - "foregroundDeletion" tells k8s we want children cleaned up
	if instance.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(instance.ObjectMeta.Finalizers, finalizerName) {
			logger.Info(fmt.Sprintf("AddFinalizer for %v", req.NamespacedName))
			instance.ObjectMeta.Finalizers = append(instance.ObjectMeta.Finalizers, finalizerName)
			if err := r.Update(ctx, instance); err != nil {
				r.Recorder.Event(instance, corev1.EventTypeWarning, "Adding finalizer", fmt.Sprintf("Failed to add finalizer: %s", err))
				TrackCronAction(CronJobDeletedMetric, false)
				return ctrl.Result{}, err
			}
			r.Recorder.Event(instance, corev1.EventTypeNormal, "Adding finalizer", "Object finalizer is added")
			TrackCronAction(CronJobDeletedMetric, true)
			return ctrl.Result{}, nil
		}
	}

	// Generate the cron we'll post and get a hash for it
	cronToPost, cronGenErr := r.generateCronJob(instance)
	if cronGenErr != nil {
		r.Recorder.Event(instance, corev1.EventTypeWarning, "Invalid cron schedule", fmt.Sprintf("Failed to generate cronjob: %s", cronGenErr))
		logger.Error(cronGenErr, "Failed to generate cronjob")
		return ctrl.Result{}, nil
	}

	objectHash, err := Hash(cronToPost, 1)
	if err != nil {
		logger.Error(err, "Failed to hash cronjob")
		return ctrl.Result{}, err
	}

	existingCron, err := r.getCronJob(ctx, cronToPost.Name, cronToPost.Namespace)

	if err != nil {
		// did we get an error because the cronjob doesn't exist?
		if errors.IsNotFound(err) {
			return r.createCronJob(ctx, cronToPost, objectHash, instance, logger)
		}

		// we hit an unexpected problem getting the cron, fail the reconcile loop
		logger.Error(err, "Failed to get associated cronjob")
		return ctrl.Result{}, err
	}

	// we found a CronJob, lets update it
	return r.updateCronJob(ctx, existingCron, cronToPost, objectHash, instance, logger)
}

func (r *PreScaledCronJobReconciler) generateCronJob(instance *pscv1alpha1.PreScaledCronJob) (*batchv1beta1.CronJob, error) {
	// Deep copy the cron
	cronToPost := instance.Spec.CronJob.DeepCopy()
	// add a label so we can watch the pods for metrics generation
	if cronToPost.Spec.JobTemplate.Spec.Template.ObjectMeta.Labels != nil {
		cronToPost.Spec.JobTemplate.Spec.Template.ObjectMeta.Labels[primedCronLabel] = instance.Name
	} else {
		cronToPost.Spec.JobTemplate.Spec.Template.ObjectMeta.Labels = map[string]string{
			primedCronLabel: instance.Name,
		}
	}

	// get original cron schedule
	scheduleSpec := instance.Spec.CronJob.Spec.Schedule
	warmUpTimeMins := instance.Spec.WarmUpTimeMins
	primerSchedule := instance.Spec.PrimerSchedule

	// Get the new schedule for the cron
	primerSchedule, err := GetPrimerSchedule(scheduleSpec, warmUpTimeMins, primerSchedule)

	if err != nil {
		return nil, fmt.Errorf("Failed parse primer schedule: %s", err)
	}

	// update cron schedule of primer cronjob
	cronToPost.Spec.Schedule = primerSchedule

	// Create + Add the init container that runs on the primed cron schedule
	// and will die on the CRONJOB_SCHEDULE
	initContainer := corev1.Container{
		Name:  warmupContainerInjectNameUID, // The warmup container has UID to allow pod controller to identify it reliably
		Image: r.InitContainerImage,
		Env: []corev1.EnvVar{
			{
				Name:  "NAMESPACE",
				Value: instance.Namespace,
			},
			{
				Name:  "CRONJOB_SCHEDULE",
				Value: scheduleSpec,
			},
		},
	}

	// set the owner reference on the autogenerated job so it's cleaned up with the parent
	ownerRef := v1.OwnerReference{
		APIVersion: instance.APIVersion,
		Kind:       instance.Kind,
		Name:       instance.Name,
		UID:        instance.UID,
	}
	cronToPost.ObjectMeta.OwnerReferences = append(cronToPost.ObjectMeta.OwnerReferences, ownerRef)

	// add the init containers to the init containers array
	cronToPost.Spec.JobTemplate.Spec.Template.Spec.InitContainers = append([]corev1.Container{initContainer}, cronToPost.Spec.JobTemplate.Spec.Template.Spec.InitContainers...)

	// Add dynamic name to cron identify one to the other
	autoGenName := "autogen-" + instance.ObjectMeta.Name
	cronToPost.ObjectMeta.Name = autoGenName
	cronToPost.ObjectMeta.Namespace = instance.ObjectMeta.Namespace

	return cronToPost, nil
}

func (r *PreScaledCronJobReconciler) getCronJob(ctx context.Context, name string, namespace string) (*batchv1beta1.CronJob, error) {
	key := client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}

	cron := batchv1beta1.CronJob{}
	err := r.Client.Get(ctx, key, &cron)
	return &cron, err
}

func (r *PreScaledCronJobReconciler) createCronJob(ctx context.Context, cronToPost *batchv1beta1.CronJob, objectHash string,
	instance *pscv1alpha1.PreScaledCronJob, logger logr.Logger) (ctrl.Result, error) {

	logger.Info(fmt.Sprintf("Creating cronjob: %v", cronToPost.ObjectMeta.Name))

	// Add the object hash as an annotation so we can compare with future updates
	if cronToPost.ObjectMeta.Annotations == nil {
		cronToPost.ObjectMeta.Annotations = map[string]string{}
	}
	cronToPost.ObjectMeta.Annotations[objectHashField] = objectHash
	if err := r.Client.Create(ctx, cronToPost); err != nil {
		r.Recorder.Event(instance, corev1.EventTypeWarning, "Create cronjob failed", fmt.Sprintf("Failed to create cronjob: %s", err))
		TrackCronAction(CronJobCreatedMetric, false)
		return ctrl.Result{}, err
	}

	r.Recorder.Event(instance, corev1.EventTypeNormal, "Create cronjob successful", fmt.Sprintf("Created associated cronjob: %s", cronToPost.Name))
	TrackCronAction(CronJobCreatedMetric, true)
	return ctrl.Result{}, nil
}

func (r *PreScaledCronJobReconciler) updateCronJob(ctx context.Context, existingCron *batchv1beta1.CronJob, cronToPost *batchv1beta1.CronJob,
	objectHash string, instance *pscv1alpha1.PreScaledCronJob, logger logr.Logger) (ctrl.Result, error) {

	logger.Info(fmt.Sprintf("Found associated cronjob: %v", existingCron.ObjectMeta.Name))

	// does this belong to us? if not - leave it alone and error out
	canUpdate := false
	for _, ref := range existingCron.ObjectMeta.OwnerReferences {
		if ref.UID == instance.UID {
			canUpdate = true
			break
		}
	}

	if !canUpdate {
		r.Recorder.Event(instance, corev1.EventTypeWarning, "Cronjob already exists", fmt.Sprintf("A cronjob with this name already exists, and was not created by this operator : %s", existingCron.ObjectMeta.Name))
		logger.Info(fmt.Sprintf("A cronjob with this name already exists, and was not created by this operator : %s", existingCron.ObjectMeta.Name))
		return ctrl.Result{}, nil
	}

	// Is it the same as what we've just generated?
	if existingCron.ObjectMeta.Annotations[objectHashField] == objectHash {
		// it's the same - no-op
		logger.Info("Autogenerated cronjob has not changed, will not recreate")
		return ctrl.Result{}, nil
	}

	// it's been updated somehow - let's update the cronjob
	existingCron.Spec = cronToPost.Spec
	if existingCron.ObjectMeta.Annotations == nil {
		existingCron.ObjectMeta.Annotations = map[string]string{}
	}

	existingCron.ObjectMeta.Annotations[objectHashField] = objectHash
	if err := r.Client.Update(ctx, existingCron); err != nil {
		r.Recorder.Event(instance, corev1.EventTypeWarning, "Update of cronjob failed", fmt.Sprintf("Failed to update cronjob: %s", err))
		logger.Error(err, "Failed to update cronjob")
		TrackCronAction(CronJobUpdatedMetric, false)
		return ctrl.Result{}, err
	}

	r.Recorder.Event(instance, corev1.EventTypeNormal, "Update of cronjob successful", fmt.Sprintf("Updated associated cronjob: %s", existingCron.Name))
	logger.Info("Successfully updated cronjob")
	TrackCronAction(CronJobUpdatedMetric, true)
	return ctrl.Result{}, nil
}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// SetupWithManager sets up defaults
func (r *PreScaledCronJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pscv1alpha1.PreScaledCronJob{}).
		Complete(r)
}
