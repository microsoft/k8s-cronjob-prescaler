package v1alpha1

import (
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PreScaledCronJobSpec defines the desired state of PreScaledCronJob
type PreScaledCronJobSpec struct {
	WarmUpTimeMins int                  `json:"warmUpTimeMins,omitempty"`
	PrimerSchedule string               `json:"primerSchedule,omitempty"`
	CronJob        batchv1beta1.CronJob `json:"cronJob,omitempty"`
}

// PreScaledCronJobStatus defines the observed state of PreScaledCronJob
type PreScaledCronJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// PreScaledCronJob is the Schema for the prescaledcronjobs API
type PreScaledCronJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreScaledCronJobSpec   `json:"spec,omitempty"`
	Status PreScaledCronJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PreScaledCronJobList contains a list of PreScaledCronJob
type PreScaledCronJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PreScaledCronJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PreScaledCronJob{}, &PreScaledCronJobList{})
}
