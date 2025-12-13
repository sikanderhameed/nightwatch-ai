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
)

// NightwatchScalerReconciler reconciles a NightwatchScaler object
type NightwatchScalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=finops.sikander.io,resources=nightwatchscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=finops.sikander.io,resources=nightwatchscalers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=finops.sikander.io,resources=nightwatchscalers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments/scale,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *NightwatchScalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Step 1: Fetch the NightwatchScaler resource
	var scaler finopsv1alpha1.NightwatchScaler
	if err := r.Get(ctx, req.NamespacedName, &scaler); err != nil {
		// Resource not found (probably deleted), ignore
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Step 2: Log what we found
	logger.Info("🔍 Reconciling NightwatchScaler",
		"name", scaler.Name,
		"namespace", scaler.Namespace,
		"targetDeployment", scaler.Spec.TargetDeployment,
	)

	// Step 3: Fetch the target Deployment
	var deployment appsv1.Deployment
	deploymentName := types.NamespacedName{
		Name:      scaler.Spec.TargetDeployment,
		Namespace: scaler.Namespace,
	}

	if err := r.Get(ctx, deploymentName, &deployment); err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "❌ Target deployment not found",
				"deployment", scaler.Spec.TargetDeployment,
			)
			// Don't requeue immediately, wait for deployment to be created
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		logger.Error(err, "Failed to get deployment")
		return ctrl.Result{}, err
	}

	// Step 4: Read current replica count
	currentReplicas := int32(0)
	if deployment.Spec.Replicas != nil {
		currentReplicas = *deployment.Spec.Replicas
	}

	logger.Info("📊 Deployment status",
		"deployment", deployment.Name,
		"currentReplicas", currentReplicas,
	)

	// Step 5: Log configuration
	if scaler.Spec.CPUThreshold != nil {
		logger.Info("📊 CPU Threshold configured",
			"threshold", *scaler.Spec.CPUThreshold,
		)
	}

	if scaler.Spec.Schedule != "" {
		logger.Info("⏰ Schedule configured",
			"schedule", scaler.Spec.Schedule,
		)
	}

	logger.Info("📈 Replica limits",
		"min", scaler.Spec.MinReplicas,
		"max", scaler.Spec.MaxReplicas,
	)

	// Step 6: Update Status fields
	scaler.Status.CurrentReplicas = currentReplicas
	scaler.Status.DesiredReplicas = currentReplicas // For now, desired = current
	now := metav1.Now()
	scaler.Status.LastScaleTime = &now

	if err := r.Status().Update(ctx, &scaler); err != nil {
		logger.Error(err, "Failed to update NightwatchScaler status")
		return ctrl.Result{}, err
	}

	logger.Info("✅ Status updated successfully",
		"currentReplicas", scaler.Status.CurrentReplicas,
		"desiredReplicas", scaler.Status.DesiredReplicas,
	)

	// Step 7: Requeue after 1 minute
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NightwatchScalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&finopsv1alpha1.NightwatchScaler{}).
		Named("nightwatchscaler").
		Complete(r)
}
