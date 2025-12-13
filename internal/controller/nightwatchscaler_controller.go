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

package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	finopsv1alpha1 "github.com/sikanderhameed/nightwatch-ai/api/v1alpha1"
	"github.com/sikanderhameed/nightwatch-ai/pkg/metrics"
)

const (
	// Annotations for state management
	AnnotationOriginalReplicas = "nightwatch.io/original-replicas"
	AnnotationSleepMode        = "nightwatch.io/sleep-mode"
)

// NightwatchScalerReconciler reconciles a NightwatchScaler object
type NightwatchScalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=finops.sikander.io,resources=nightwatchscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=finops.sikander.io,resources=nightwatchscalers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=finops.sikander.io,resources=nightwatchscalers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments/scale,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *NightwatchScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Fetch the NightwatchScaler resource
	var scaler finopsv1alpha1.NightwatchScaler
	if err := r.Get(ctx, req.NamespacedName, &scaler); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("🔍 Reconciling NightwatchScaler",
		"name", scaler.Name,
		"namespace", scaler.Namespace,
		"targetDeployment", scaler.Spec.TargetDeployment,
	)

	// Step 2: Fetch the target Deployment
	var deployment appsv1.Deployment
	deploymentName := types.NamespacedName{
		Name:      scaler.Spec.TargetDeployment,
		Namespace: scaler.Namespace,
	}

	if err := r.Get(ctx, deploymentName, &deployment); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "❌ Target deployment not found", "deployment", scaler.Spec.TargetDeployment)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		logger.Error(err, "Failed to get deployment")
		return ctrl.Result{}, err
	}

	// Step 3: Read current replica count
	currentReplicas := int32(0)
	if deployment.Spec.Replicas != nil {
		currentReplicas = *deployment.Spec.Replicas
	}

	logger.Info("📊 Deployment status", "deployment", deployment.Name, "currentReplicas", currentReplicas)

	// Step 4: Determine desired replica count based on sleep schedule
	desiredReplicas := currentReplicas
	inSleepMode := false

	// Get min/max replicas
	minReplicas := int32(0)
	if scaler.Spec.MinReplicas != nil {
		minReplicas = *scaler.Spec.MinReplicas
	}

	maxReplicas := int32(10)
	if scaler.Spec.MaxReplicas != nil {
		maxReplicas = *scaler.Spec.MaxReplicas
	}

	// PRIORITY 1: Queue Depth (Business Critical - User Experience)
	// Check vLLM queue depth with circuit breaker pattern
	if scaler.Spec.VLLMMetricsURL != "" && scaler.Spec.QueueDepthThreshold != nil {
		queueDepth, err := metrics.FetchVLLMQueueDepth(scaler.Spec.VLLMMetricsURL)
		
		if err != nil {
			// METRICS FETCH FAILED - Circuit Breaker Logic
			scaler.Status.MetricsFailureCount++
			
			logger.Error(err, "Failed to fetch queue depth",
				"url", scaler.Spec.VLLMMetricsURL,
				"failureCount", scaler.Status.MetricsFailureCount,
			)
			
			if scaler.Status.MetricsFailureCount < 3 {
				// Tier 1-2: Use Last Known Good Value
				if scaler.Status.LastKnownQueueDepth != nil {
					queueDepth = int(*scaler.Status.LastKnownQueueDepth)
					logger.Info("⚠️ Using last known queue depth (metrics unavailable)",
						"queueDepth", queueDepth,
						"failureCount", scaler.Status.MetricsFailureCount,
					)
				} else {
					// No LKGV available, assume high load (fail-safe)
					queueDepth = int(*scaler.Spec.QueueDepthThreshold) + 1
					logger.Info("⚠️ No last known value, assuming high load (fail-safe)",
						"queueDepth", queueDepth,
					)
				}
			} else {
				// Tier 3: CIRCUIT BREAKER TRIPPED
				scaler.Status.CircuitBreakerTripped = true
				
				logger.Error(err, "🚨 CIRCUIT BREAKER TRIPPED - Scaling to maxReplicas (fail-safe)",
					"failureCount", scaler.Status.MetricsFailureCount,
					"maxReplicas", maxReplicas,
				)
				
				// Fail-safe: Scale to max to prevent outage
				desiredReplicas = maxReplicas
				inSleepMode = false
				goto ScaleDecisionMade
			}
		} else {
			// METRICS FETCH SUCCEEDED - Reset circuit breaker
			scaler.Status.MetricsFailureCount = 0
			scaler.Status.CircuitBreakerTripped = false
			
			// Update Last Known Good Value
			queueDepth32 := int32(queueDepth)
			scaler.Status.LastKnownQueueDepth = &queueDepth32
			scaler.Status.LastQueueDepth = &queueDepth32
			
			logger.Info("📊 Queue depth fetched successfully",
				"queueDepth", queueDepth,
				"threshold", *scaler.Spec.QueueDepthThreshold,
			)
		}
		
		// Continue with normal queue depth logic (if not circuit breaker)
		if queueDepth > int(*scaler.Spec.QueueDepthThreshold) {
			// Calculate desired replicas based on queue depth
			// Simple algorithm: 1 replica per 3 requests in queue, capped at maxReplicas
			desiredReplicas = int32((queueDepth / 3) + 1)
			if desiredReplicas > maxReplicas {
				desiredReplicas = maxReplicas
			}
			if desiredReplicas < minReplicas {
				desiredReplicas = minReplicas
			}
			
			logger.Info("🚨 High queue depth detected, scaling up",
				"queueDepth", queueDepth,
				"threshold", *scaler.Spec.QueueDepthThreshold,
				"desiredReplicas", desiredReplicas,
				"overridingSleepMode", true,
			)
			
			inSleepMode = false // Override sleep mode
			goto ScaleDecisionMade
		}
	}


	// PRIORITY 2: Sleep Schedule (Cost Optimization)
	// Check if sleepSchedule is configured
	if scaler.Spec.SleepSchedule != "" {
		startHour, endHour, err := parseSleepSchedule(scaler.Spec.SleepSchedule)
		if err != nil {
			logger.Error(err, "Failed to parse sleep schedule", "schedule", scaler.Spec.SleepSchedule)
		} else {
			inSleepWindow := isInSleepWindow(startHour, endHour)

			if inSleepWindow {
				// SLEEP MODE: Scale to 0 and save state
				logger.Info("💤 Sleep mode active", "schedule", scaler.Spec.SleepSchedule, "window", fmt.Sprintf("%02d:00-%02d:00", startHour, endHour))
				
				// Save current replicas to annotation if not already in sleep mode
				if deployment.Annotations == nil {
					deployment.Annotations = make(map[string]string)
				}
				
				if deployment.Annotations[AnnotationSleepMode] != "true" && currentReplicas > 0 {
					deployment.Annotations[AnnotationOriginalReplicas] = strconv.Itoa(int(currentReplicas))
					deployment.Annotations[AnnotationSleepMode] = "true"
					logger.Info("💾 Saving state before sleep", "originalReplicas", currentReplicas)
				}
				
				desiredReplicas = 0
				inSleepMode = true
			} else {
				// WAKE MODE: Restore from annotation if exists
				logger.Info("☀️ Wake mode active", "schedule", scaler.Spec.SleepSchedule)
				
				if deployment.Annotations != nil && deployment.Annotations[AnnotationSleepMode] == "true" {
					if originalStr, exists := deployment.Annotations[AnnotationOriginalReplicas]; exists {
						original, _ := strconv.Atoi(originalStr)
						desiredReplicas = int32(original)
						logger.Info("🔄 Restoring from sleep", "originalReplicas", desiredReplicas)
						
						// Clean up annotations after restoring
						delete(deployment.Annotations, AnnotationOriginalReplicas)
						delete(deployment.Annotations, AnnotationSleepMode)
					}
				}
				inSleepMode = false
			}
		}
	} else {
		// No sleep schedule - use Phase 3 logic
		if scaler.Spec.CPUThreshold == nil && scaler.Spec.Schedule == "" && scaler.Spec.VLLMMetricsURL == "" {
			desiredReplicas = minReplicas
			logger.Info("⚙️ No threshold/schedule configured, scaling to minReplicas", "minReplicas", minReplicas)
		} else {
			if scaler.Spec.CPUThreshold != nil {
				logger.Info("📊 CPU Threshold configured", "threshold", *scaler.Spec.CPUThreshold)
			}
			if scaler.Spec.Schedule != "" {
				logger.Info("⏰ Schedule configured", "schedule", scaler.Spec.Schedule)
			}
		}
	}

ScaleDecisionMade:
	logger.Info("📈 Replica limits", "min", minReplicas, "max", maxReplicas)

	// Step 5: Scale if needed
	if desiredReplicas != currentReplicas {
		logger.Info("🔄 Scaling deployment", "from", currentReplicas, "to", desiredReplicas)

		deployment.Spec.Replicas = &desiredReplicas
		if err := r.Update(ctx, &deployment); err != nil {
			logger.Error(err, "Failed to scale deployment")
			return ctrl.Result{}, err
		}

		logger.Info("✅ Deployment scaled successfully")
	}

	// Step 6: Update Status fields
	scaler.Status.CurrentReplicas = currentReplicas
	scaler.Status.DesiredReplicas = desiredReplicas
	scaler.Status.SleepMode = inSleepMode
	now := metav1.Now()
	scaler.Status.LastScaleTime = &now

	if err := r.Status().Update(ctx, &scaler); err != nil {
		logger.Error(err, "Failed to update NightwatchScaler status")
		return ctrl.Result{}, err
	}

	logger.Info("✅ Status updated successfully",
		"currentReplicas", scaler.Status.CurrentReplicas,
		"desiredReplicas", scaler.Status.DesiredReplicas,
		"sleepMode", scaler.Status.SleepMode,
	)

	// Step 7: Requeue after 1 minute
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// parseSleepSchedule parses a sleep schedule in format "HH:MM-HH:MM"
func parseSleepSchedule(schedule string) (startHour, endHour int, err error) {
	parts := strings.Split(schedule, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid schedule format, expected HH:MM-HH:MM")
	}

	start, err := time.Parse("15:04", parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start time: %w", err)
	}

	end, err := time.Parse("15:04", parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end time: %w", err)
	}

	return start.Hour(), end.Hour(), nil
}

// isInSleepWindow checks if current time is within the sleep window
func isInSleepWindow(startHour, endHour int) bool {
	now := time.Now().UTC()
	currentHour := now.Hour()

	// Handle overnight schedules (e.g., 20:00-08:00)
	if startHour > endHour {
		return currentHour >= startHour || currentHour < endHour
	}

	// Handle same-day schedules (e.g., 01:00-05:00)
	return currentHour >= startHour && currentHour < endHour
}

// SetupWithManager sets up the controller with the Manager.
func (r *NightwatchScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&finopsv1alpha1.NightwatchScaler{}).
		Named("nightwatchscaler").
		Complete(r)
}
