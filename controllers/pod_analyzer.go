package controllers

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sujall18/kubeMedic/observability"
)

func AnalyzePod(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
	tracker *IncidentTracker,
) {

	for _, containerStatus := range pod.Status.ContainerStatuses {

		key := incidentKey(
			ctx,
			clientset,
			pod,
		)

		// One workload = one active incident.
		// Ignore all subsequent Pod events while remediation is running.
		if !tracker.Start(key) {
			return
		}

		if containerStatus.State.Waiting == nil {
			tracker.Resolve(key)
			continue
		}

		reason :=
			containerStatus.State.Waiting.Reason

		if reason != "CrashLoopBackOff" &&
			reason != "ImagePullBackOff" &&
			reason != "ErrImagePull" {

			tracker.Resolve(key)
			continue
		}

		observability.IncidentsDetected.
			WithLabelValues(reason).
			Inc()

		observability.ActiveIncidents.Inc()

		defer observability.ActiveIncidents.Dec()

		fmt.Println()
		fmt.Println("🚨 INCIDENT DETECTED")
		fmt.Println("----------------------------------------")

		fmt.Printf(
			"Pod: %s/%s\n",
			pod.Namespace,
			pod.Name,
		)

		fmt.Printf(
			"Container: %s\n",
			containerStatus.Name,
		)

		fmt.Printf(
			"Status: %s\n",
			reason,
		)

		fmt.Printf(
			"Restarts: %d\n",
			containerStatus.RestartCount,
		)

		logs, err :=
			clientset.
				CoreV1().
				Pods(pod.Namespace).
				GetLogs(
					pod.Name,
					&corev1.PodLogOptions{
						Container: containerStatus.Name,
						Previous:  true,
					},
				).
				Do(ctx).
				Raw()

		logText := ""

		if err != nil {

			fmt.Printf(
				"Could not retrieve logs: %v\n",
				err,
			)

		} else {

			logText = string(logs)

			fmt.Printf(
				"Previous logs:\n%s\n",
				logText,
			)
		}

		diagnosis :=
			DiagnoseContainer(
				containerStatus,
				logText,
			)

		// CrashLoop regression detection.
		if reason == "CrashLoopBackOff" {

			if regression, err :=
				DiagnoseDeploymentRegression(
					ctx,
					clientset,
					pod,
				); err == nil && regression != nil {

				diagnosis = *regression
			}
		}

		fmt.Println()
		fmt.Println("Diagnosis:")
		fmt.Printf(
			"  Problem: %s\n",
			diagnosis.Problem,
		)
		fmt.Printf(
			"  Reason: %s\n",
			diagnosis.Reason,
		)
		fmt.Printf(
			"  Severity: %s\n",
			diagnosis.Severity,
		)
		fmt.Printf(
			"  Recommended action: %s\n",
			diagnosis.Action,
		)

		fmt.Println("----------------------------------------")

		// KubeMedic does not automatically modify workloads
		// unless the diagnosis has an explicit remediation policy.
		if !ShouldRemediate(diagnosis) {

			fmt.Println(
				"⏸️ No automatic remediation selected by policy",
			)

			tracker.Resolve(key)

			continue
		}

		fmt.Println()
		fmt.Println(
			"🤖 KubeMedic will attempt automatic remediation",
		)
		fmt.Println()

		// ============================================================
		// OOM REMEDIATION
		// ============================================================

		if diagnosis.Reason == DiagnosisOOMKilled {

			fmt.Println(
				"⚕️ REMEDIATION: Automatic memory escalation",
			)

			result, err :=
				RemediateOOM(
					ctx,
					clientset,
					pod,
				)

			if err != nil {

				fmt.Printf(
					"❌ OOM remediation failed: %v\n",
					err,
				)

				tracker.Exhaust(key)

				fmt.Println(
					"🛑 Automatic remediation stopped",
				)

				return
			}

			if result.Resolved {

				fmt.Printf(
					"Deployment healthy after memory remediation: %s\n",
					result.LastMemoryLimit,
				)

				fmt.Println(
					"✅ INCIDENT RESOLVED",
				)

				tracker.Resolve(key)

				return
			}

			if result.Exhausted {

				fmt.Printf(
					"❌ INCIDENT UNRESOLVED: %s\n",
					result.Message,
				)

				fmt.Printf(
					"🛑 Maximum automatic memory limit: %s\n",
					MaxAutomaticMemoryLimit,
				)

				tracker.Exhaust(key)

				return
			}
		}

		// ============================================================
		// ROLLBACK REMEDIATION
		// ============================================================

		result, err :=
			RemediateCrashLoop(
				ctx,
				clientset,
				pod,
				RemediationPolicy{
					AutomaticRollback: true,
				},
			)

		if err != nil {

			fmt.Printf(
				"❌ Remediation failed: %v\n",
				err,
			)

			tracker.Exhaust(key)

			return
		}

		fmt.Printf(
			"Rollback: revision %d → %d\n",
			result.FromRevision,
			result.ToRevision,
		)

		fmt.Printf(
			"Image: %s → %s\n",
			result.CurrentImage,
			result.PreviousImage,
		)

		fmt.Println(
			"⏳ Waiting for deployment recovery...",
		)

		verifyCtx, cancel :=
			context.WithTimeout(
				ctx,
				90*time.Second,
			)

		err =
			WaitForDeploymentHealthy(
				verifyCtx,
				clientset,
				pod.Namespace,
				result.Deployment,
			)

		cancel()

		if err != nil {

			fmt.Printf(
				"❌ INCIDENT UNRESOLVED: %v\n",
				err,
			)
			observability.VerificationTotal.
				WithLabelValues("failure").
				Inc()

			observability.IncidentsExhausted.
				WithLabelValues(reason).
				Inc()
			tracker.Exhaust(key)

			return
		}

		fmt.Printf(
			"Deployment healthy: %d/%d pods Ready\n",
			result.DesiredReplicas,
			result.DesiredReplicas,
		)

		fmt.Println(
			"✅ INCIDENT RESOLVED",
		)

		tracker.Resolve(key)

		return
	}
}

func incidentKey(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) string {
	deployment, err := getOwningDeployment(ctx, clientset, pod)

	if err != nil {
		// Fallback for pods that aren't owned by a Deployment.
		return fmt.Sprintf("%s/pod/%s", pod.Namespace, pod.Name)
	}

	return fmt.Sprintf(
		"%s/deployment/%s",
		deployment.Namespace,
		deployment.Name,
	)
}

func getOwningDeployment(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) (*appsv1.Deployment, error) {

	// Pod → ReplicaSet
	var rsName string

	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "ReplicaSet" &&
			ref.Controller != nil &&
			*ref.Controller {

			rsName = ref.Name
			break
		}
	}

	if rsName == "" {
		return nil, fmt.Errorf(
			"pod %s/%s has no owning ReplicaSet",
			pod.Namespace,
			pod.Name,
		)
	}

	rs, err := clientset.
		AppsV1().
		ReplicaSets(pod.Namespace).
		Get(
			ctx,
			rsName,
			metav1.GetOptions{},
		)

	if err != nil {
		return nil, fmt.Errorf(
			"get ReplicaSet %s: %w",
			rsName,
			err,
		)
	}

	// ReplicaSet → Deployment
	for _, ref := range rs.OwnerReferences {
		if ref.Kind == "Deployment" &&
			ref.Controller != nil &&
			*ref.Controller {

			deployment, err := clientset.
				AppsV1().
				Deployments(pod.Namespace).
				Get(
					ctx,
					ref.Name,
					metav1.GetOptions{},
				)

			if err != nil {
				return nil, fmt.Errorf(
					"get Deployment %s: %w",
					ref.Name,
					err,
				)
			}

			return deployment, nil
		}
	}

	return nil, fmt.Errorf(
		"ReplicaSet %s has no owning Deployment",
		rs.Name,
	)
}

func deploymentNameFromPod(pod *corev1.Pod) string {
	for _, ref := range pod.OwnerReferences {
		if ref.Kind == "ReplicaSet" && ref.Controller != nil && *ref.Controller {
			return ref.Name
		}
	}
	return pod.Name
}
