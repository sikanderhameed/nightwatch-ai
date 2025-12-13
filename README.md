# Nightwatch AI Operator

**AI-aware FinOps controller for Kubernetes**

[![Go Version](https://img.shields.io/badge/Go-1.24-blue.svg)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.28+-326CE5.svg)](https://kubernetes.io)
[![License](https://img.shields.io/badge/License-Apache%202.0-green.svg)](LICENSE)

> A custom Kubernetes operator that combines business metrics with cost optimization through intelligent, priority-based autoscaling with production-grade fault tolerance.

---

## 🎯 The Business Problem

**Challenge:** Standard autoscalers (HPA, KEDA) cannot combine business metrics with cost policies in a single decision loop.

**Impact:** Organizations must choose between:
- ❌ User experience (scale based on demand)
- ❌ Cost optimization (scale down during off-hours)

**Result:** Either degraded performance or wasted compute costs.

---

## 💡 The Solution: Intelligent Multi-Signal Autoscaling

Nightwatch AI is a **custom Kubernetes operator** that implements priority-based scaling:

```
Priority 1: Queue Depth (User Experience)
  ↓ If AI workload queue > threshold → Scale up immediately
  
Priority 2: Sleep Schedule (Cost Optimization)
  ↓ If in off-hours window → Scale to 0
  
Priority 3: Circuit Breaker (Resilience)
  ↓ If metrics unavailable → Fail-safe to maxReplicas
```

**This enables AI-aware FinOps** - optimizing costs without sacrificing user experience.

---

## 🏗️ Architecture

### System Design

```
┌──────────────────────────────────────────────────────┐
│  Nightwatch AI Operator (Custom Controller)          │
│                                                       │
│  ┌────────────────────────────────────────────────┐ │
│  │  Priority-Based Decision Engine                │ │
│  │                                                 │ │
│  │  1. Fetch vLLM Queue Depth (HTTP + Prometheus) │ │
│  │  2. Check Circuit Breaker Status               │ │
│  │  3. Evaluate Sleep Schedule                    │ │
│  │  4. Calculate Desired Replicas                 │ │
│  │  5. Update Deployment + Status                 │ │
│  └────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────┘
           ↓                           ↓
    ┌──────────┐              ┌─────────────────┐
    │  vLLM    │              │  Kubernetes     │
    │  Metrics │              │  Deployments    │
    └──────────┘              └─────────────────┘
```

### Custom Resource Definition (CRD)

```yaml
apiVersion: finops.sikander.io/v1alpha1
kind: NightwatchScaler
metadata:
  name: ai-inference-scaler
spec:
  targetDeployment: vllm-inference
  
  # Business Metric (Priority 1)
  queueDepthThreshold: 5
  vllmMetricsURL: "http://vllm:8000/metrics"
  
  # Cost Optimization (Priority 2)
  sleepSchedule: "20:00-08:00"  # Sleep 8 PM to 8 AM
  
  # Constraints
  minReplicas: 0
  maxReplicas: 10
```

---

## 🚀 Key Features

### 1. **AI-Aware Autoscaling**
- Fetches real-time queue depth from vLLM metrics endpoint
- Scales based on actual business demand, not just CPU/memory
- Overrides cost policies when user experience is at risk

### 2. **Production-Grade Resilience**
- **Circuit Breaker Pattern**: 3-tier graceful degradation
  - Tier 1-2: Use Last Known Good Value (LKGV)
  - Tier 3: Fail-safe to `maxReplicas` (not down!)
- **Fail-Safe Design**: Prefers over-provisioning to outages
- **State Management**: Uses Kubernetes annotations for distributed state

### 3. **Time-Based FinOps**
- Automatic sleep/wake cycles for cost savings
- Saves original replica count before sleeping
- Restores state on wake-up
- **Business Value**: "Saved $X,000/month by auto-sleeping dev environments"

### 4. **Priority-Based Decision Making**
```go
// Queue depth overrides sleep schedule
if queueDepth > threshold {
    scale_up()  // User experience first
} else if in_sleep_window {
    scale_to_zero()  // Cost optimization second
}
```

---

## 📊 Technical Highlights

### Code Statistics

- **Language**: Go 1.24
- **Framework**: Kubebuilder v4 / controller-runtime
- **Lines of Code**: ~500 (controller + CRD)
- **Docker Images**: 6 versions (v0.1.0 → v0.6.0)
- **Deployment**: Amazon EKS

---

## 🔬 Proof of Work

### Circuit Breaker in Action

**Scenario**: vLLM metrics service goes down

```
📊 Queue depth fetched successfully (queueDepth=15)
⚠️ Failed to fetch queue depth (failureCount=1)
⚠️ Using last known queue depth (queueDepth=15, LKGV)
⚠️ Failed to fetch queue depth (failureCount=2)
⚠️ Using last known queue depth (queueDepth=15, LKGV)
🚨 CIRCUIT BREAKER TRIPPED - Scaling to maxReplicas (fail-safe)
✅ Scaled deployment from 0 → 10 replicas (prevented outage!)
```

### AI-Aware Scaling

**Scenario**: High AI workload demand during sleep window

```
💤 Sleep mode active (schedule=20:00-08:00)
📊 Queue depth fetched (queueDepth=15, threshold=5)
🚨 High queue depth detected, scaling up
   overridingSleepMode=true
🔄 Scaling deployment from 0 → 6 replicas
✅ User experience prioritized over cost savings
```

---

## 🛠️ Technology Stack

**Core:**
- Go 1.24
- Kubebuilder v4.10.1
- controller-runtime
- client-go

**Infrastructure:**
- Kubernetes 1.28+
- AWS EKS
- Amazon ECR
- Terraform

**Observability:**
- Prometheus metrics
- Kubernetes Events API
- Structured logging

---

## Compared with KEDA and HPA

**What KEDA/HPA can do:**
- Scale based on CPU
- Scale based on queue depth
- Scale based on schedule

**What they CANNOT do:**
- Combine multiple signals with priority
- Override cost policies for user experience
- Implement custom business logic
- Design failure states (fail-safe vs fail-dangerous)

**Nightwatch AI does all of this.**

---

## 🚀 Quick Start

```bash
# Deploy the operator
kubectl apply -f config/crd/bases/finops.sikander.io_nightwatchscalers.yaml
kubectl apply -f config/rbac/
kubectl apply -f config/manager/

# Create a scaler
kubectl apply -f examples/ai-aware-scaler.yaml

# Watch it work
kubectl get nightwatchscaler -w
kubectl logs -n nightwatch-system -l control-plane=controller-manager -f
```

---

## 📄 License

Apache License 2.0


