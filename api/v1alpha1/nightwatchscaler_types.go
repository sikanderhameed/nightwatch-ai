/*
Copyright 2025.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NightwatchScalerSpec defines the desired state of NightwatchScaler
type NightwatchScalerSpec struct {
	// TargetDeployment is the name of the Deployment to scale
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TargetDeployment string `json:"targetDeployment"`

	// CPUThreshold is the minimum CPU utilization percentage (0-100)
	// If CPU utilization falls below this threshold, the deployment will be scaled down
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	CPUThreshold *int32 `json:"cpuThreshold,omitempty"`

	// Schedule is a cron expression for time-based scaling (e.g., "0 20 * * *" for 8pm daily)
	// +optional
	Schedule string `json:"schedule,omitempty"`

	// SleepSchedule defines when to scale down to save costs (FinOps)
	// Format: "HH:MM-HH:MM" (e.g., "20:00-08:00" means sleep from 8 PM to 8 AM)
	// During sleep window, deployment scales to 0 and original replica count is saved
	// +kubebuilder:validation:Pattern=`^([0-1][0-9]|2[0-3]):[0-5][0-9]-([0-1][0-9]|2[0-3]):[0-5][0-9]$`
	// +optional
	SleepSchedule string `json:"sleepSchedule,omitempty"`

	// MinReplicas is the minimum number of replicas (default: 0)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=0
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the maximum number of replicas (default: 10)
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// QueueDepthThreshold is the maximum acceptable queue depth
	// If queue depth exceeds this, scale up immediately (overrides sleep mode)
	// +kubebuilder:validation:Minimum=0
	// +optional
	QueueDepthThreshold *int32 `json:"queueDepthThreshold,omitempty"`

	// VLLMMetricsURL is the URL to fetch vLLM metrics from
	// Example: "http://mock-vllm:8000/metrics"
	// +optional
	VLLMMetricsURL string `json:"vllmMetricsURL,omitempty"`
}

// NightwatchScalerStatus defines the observed state of NightwatchScaler.
type NightwatchScalerStatus struct {
	// CurrentReplicas is the current number of replicas
	// +optional
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`

	// DesiredReplicas is the desired number of replicas based on scaling logic
	// +optional
	DesiredReplicas int32 `json:"desiredReplicas,omitempty"`

	// LastScaleTime is the last time the deployment was scaled
	// +optional
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// LastCPUUtilization is the last observed CPU utilization percentage
	// +optional
	LastCPUUtilization *int32 `json:"lastCPUUtilization,omitempty"`

	// SleepMode indicates if the deployment is currently in sleep mode
	// +optional
	SleepMode bool `json:"sleepMode,omitempty"`

	// LastQueueDepth is the last observed vLLM queue depth
	// +optional
	LastQueueDepth *int32 `json:"lastQueueDepth,omitempty"`

	// LastKnownQueueDepth is the last successfully fetched queue depth
	// Used for graceful degradation when metrics service is unavailable
	// +optional
	LastKnownQueueDepth *int32 `json:"lastKnownQueueDepth,omitempty"`

	// MetricsFailureCount tracks consecutive metrics fetch failures
	// Circuit breaker trips at 3 failures
	// +optional
	MetricsFailureCount int32 `json:"metricsFailureCount,omitempty"`

	// CircuitBreakerTripped indicates if the circuit breaker is active
	// When true, operator scales to maxReplicas for safety
	// +optional
	CircuitBreakerTripped bool `json:"circuitBreakerTripped,omitempty"`

	// conditions represent the current state of the NightwatchScaler resource.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=`.spec.targetDeployment`
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.currentReplicas`
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.status.desiredReplicas`
// +kubebuilder:printcolumn:name="Queue",type=integer,JSONPath=`.status.lastQueueDepth`
// +kubebuilder:printcolumn:name="Sleep",type=boolean,JSONPath=`.status.sleepMode`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NightwatchScaler is the Schema for the nightwatchscalers API
type NightwatchScaler struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of NightwatchScaler
	// +required
	Spec NightwatchScalerSpec `json:"spec"`

	// status defines the observed state of NightwatchScaler
	// +optional
	Status NightwatchScalerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// NightwatchScalerList contains a list of NightwatchScaler
type NightwatchScalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []NightwatchScaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NightwatchScaler{}, &NightwatchScalerList{})
}
