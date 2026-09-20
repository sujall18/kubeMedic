package controllers

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/sujall18/kubeMedic/observability"
)

const deploymentRevisionAnnotation = "deployment.kubernetes.io/revision"

const (
	DiagnosisDeploymentRegression = "DEPLOYMENT_REGRESSION"
	RemediationRollback           = "ROLLBACK"
)

const (
	RemediationIncreaseMemory = "INCREASE_MEMORY"
	MaxAutomaticMemoryLimit   = "256Mi"
)

type RemediationPolicy struct {
	AutomaticRollback bool
}

const MaxAutomaticMemoryAttempts = 2

type OOMRemediationResult struct {
	Deployment      string
	Attempts        int
	Resolved        bool
	Exhausted       bool
	LastMemoryLimit string
	Message         string
}

type RemediationResult struct {
	Action          string
	Deployment      string
	Automatic       bool
	FromRevision    int64
	ToRevision      int64
	CurrentImage    string
	PreviousImage   string
	DesiredReplicas int32
	Executed        bool
	Message         string
}

// ShouldRemediate is deliberately conservative. We do not automatically
// modify a workload for every diagnosis. Only a diagnosis with an explicit
// safe remediation policy reaches the remediation engine.
func ShouldRemediate(diagnosis Diagnosis) bool {
	switch diagnosis.Reason {
	case DiagnosisDeploymentRegression,
		DiagnosisOOMKilled,
		DiagnosisImagePullFailure:
		//	DiagnosisOOMKilled:
		return true
	default:
		return false
	}
}

// RemediateCrashLoop attempts the safest remediation we currently support:
// rolling a failed Deployment back to its immediately previous revision.
func RemediateCrashLoop(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
	policy RemediationPolicy,
) (result RemediationResult, err error) {

	start := time.Now()
	observability.ActiveIncidents.Inc()

	defer func() {
		observability.RemediationDuration.
			WithLabelValues(RemediationRollback).
			Observe(time.Since(start).Seconds())

		observability.ActiveIncidents.Dec()

		if err != nil {
			observability.RemediationsTotal.
				WithLabelValues(RemediationRollback, "failure").
				Inc()
			return
		}

		if result.Executed {
			observability.RemediationsTotal.
				WithLabelValues(RemediationRollback, "success").
				Inc()
			return
		}

		observability.RemediationsTotal.
			WithLabelValues(RemediationRollback, "skipped").
			Inc()
	}()

	if !policy.AutomaticRollback {
		result.Message = "automatic rollback disabled by policy"
		return result, nil
	}

	rs, deployment, err := owningDeployment(ctx, clientset, pod)
	if err != nil {
		return result, err
	}

	result.Deployment = deployment.Name

	currentRevision, err := revisionOf(rs)
	if err != nil {
		return result, fmt.Errorf("current ReplicaSet revision: %w", err)
	}
	result.FromRevision = currentRevision

	previous, err := previousReplicaSet(ctx, clientset, deployment, currentRevision)
	if err != nil {
		return result, err
	}

	previousRevision, err := revisionOf(previous)
	if err != nil {
		return result, fmt.Errorf("previous ReplicaSet revision: %w", err)
	}
	result.ToRevision = previousRevision

	currentImage := firstContainerImage(deployment.Spec.Template.Spec.Containers)
	previousImage := firstContainerImage(previous.Spec.Template.Spec.Containers)
	result.CurrentImage = currentImage
	result.PreviousImage = previousImage
	if deployment.Spec.Replicas != nil {
		result.DesiredReplicas = *deployment.Spec.Replicas
	} else {
		result.DesiredReplicas = 1
	}

	// The Kubernetes API does not retain historical readiness for a scaled-down
	// ReplicaSet. Therefore this first remediation implementation treats the
	// immediately previous revision as the rollback target, while requiring it
	// to exist and have a lower revision number. A later health-history store can
	// strengthen this check using KubeMedic's own observations.
	if previousRevision >= currentRevision {
		return result, fmt.Errorf("unsafe rollback target: revision %d is not older than %d", previousRevision, currentRevision)
	}

	deployment.Spec.Template = *previous.Spec.Template.DeepCopy()
	if deployment.Annotations == nil {
		deployment.Annotations = map[string]string{}
	}
	deployment.Annotations["kubemedic.io/rollback-from"] = strconv.FormatInt(currentRevision, 10)
	deployment.Annotations["kubemedic.io/rollback-to"] = strconv.FormatInt(previousRevision, 10)

	_, err = clientset.AppsV1().Deployments(deployment.Namespace).Update(
		ctx,
		deployment,
		metav1.UpdateOptions{},
	)
	if err != nil {
		return result, fmt.Errorf("update deployment for rollback: %w", err)
	}

	result.Executed = true
	result.Message = fmt.Sprintf("rollback initiated: revision %d -> %d", currentRevision, previousRevision)
	return result, nil
}

func owningDeployment(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) (*appsv1.ReplicaSet, *appsv1.Deployment, error) {
	var rsRef *metav1.OwnerReference
	for i := range pod.OwnerReferences {
		ref := pod.OwnerReferences[i]
		if ref.Kind == "ReplicaSet" && ref.Controller != nil && *ref.Controller {
			refCopy := ref
			rsRef = &refCopy
			break
		}
	}
	if rsRef == nil {
		return nil, nil, fmt.Errorf("pod %s/%s is not owned by a ReplicaSet", pod.Namespace, pod.Name)
	}

	rs, err := clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, rsRef.Name, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get owning ReplicaSet %s: %w", rsRef.Name, err)
	}

	var deploymentName string
	for _, ref := range rs.OwnerReferences {
		if ref.Kind == "Deployment" && ref.Controller != nil && *ref.Controller {
			deploymentName = ref.Name
			break
		}
	}
	if deploymentName == "" {
		return nil, nil, fmt.Errorf("ReplicaSet %s has no owning Deployment", rs.Name)
	}

	deployment, err := clientset.AppsV1().Deployments(pod.Namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get owning Deployment %s: %w", deploymentName, err)
	}

	return rs, deployment, nil
}

func previousReplicaSet(
	ctx context.Context,
	clientset kubernetes.Interface,
	deployment *appsv1.Deployment,
	currentRevision int64,
) (*appsv1.ReplicaSet, error) {
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("build Deployment selector: %w", err)
	}

	rss, err := clientset.AppsV1().ReplicaSets(deployment.Namespace).List(
		ctx,
		metav1.ListOptions{LabelSelector: selector.String()},
	)
	if err != nil {
		return nil, fmt.Errorf("list ReplicaSets: %w", err)
	}

	candidates := make([]*appsv1.ReplicaSet, 0)
	for i := range rss.Items {
		rs := &rss.Items[i]
		rev, err := revisionOf(rs)
		if err != nil || rev >= currentRevision {
			continue
		}
		candidates = append(candidates, rs)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no previous ReplicaSet revision found before %d", currentRevision)
	}

	sort.Slice(candidates, func(i, j int) bool {
		a, _ := revisionOf(candidates[i])
		b, _ := revisionOf(candidates[j])
		return a > b
	})

	return candidates[0], nil
}

func revisionOf(rs *appsv1.ReplicaSet) (int64, error) {
	if rs.Annotations == nil {
		return 0, fmt.Errorf("ReplicaSet %s has no revision annotation", rs.Name)
	}
	revision := rs.Annotations[deploymentRevisionAnnotation]
	if revision == "" {
		return 0, fmt.Errorf("ReplicaSet %s has no revision annotation", rs.Name)
	}
	value, err := strconv.ParseInt(revision, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid revision %q on ReplicaSet %s", revision, rs.Name)
	}
	return value, nil
}

func firstContainerImage(containers []corev1.Container) string {
	if len(containers) == 0 {
		return ""
	}
	return containers[0].Image
}

// DiagnoseDeploymentRegression returns a diagnosis only when there is enough
// Deployment history to identify a newer failing revision and an older
// revision with a different image. It intentionally does not claim that the
// older revision was healthy historically; Kubernetes does not retain that
// readiness history on scaled-down ReplicaSets.
func DiagnoseDeploymentRegression(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) (*Diagnosis, error) {
	rs, deployment, err := owningDeployment(ctx, clientset, pod)
	if err != nil {
		return nil, err
	}

	currentRevision, err := revisionOf(rs)
	if err != nil {
		return nil, err
	}

	previous, err := previousReplicaSet(ctx, clientset, deployment, currentRevision)
	if err != nil {
		return nil, err
	}

	previousRevision, err := revisionOf(previous)
	if err != nil {
		return nil, err
	}

	currentImage := firstContainerImage(deployment.Spec.Template.Spec.Containers)
	previousImage := firstContainerImage(previous.Spec.Template.Spec.Containers)
	if currentImage == "" || previousImage == "" || currentImage == previousImage {
		return nil, nil
	}

	return &Diagnosis{
		Problem:  "Failed deployment",
		Reason:   DiagnosisDeploymentRegression,
		Severity: "CRITICAL",
		Action:   fmt.Sprintf("Rollback revision %d to revision %d", currentRevision, previousRevision),
	}, nil
}

// WaitForDeploymentHealthy verifies the result of a remediation through the
// Deployment status reported by the Kubernetes control plane.
func WaitForDeploymentHealthy(
	ctx context.Context,
	clientset kubernetes.Interface,
	namespace string,
	deploymentName string,
) error {

	const pollInterval = 2 * time.Second

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {

		deployment, err :=
			clientset.
				AppsV1().
				Deployments(namespace).
				Get(
					ctx,
					deploymentName,
					metav1.GetOptions{},
				)

		if err != nil {

			fmt.Printf(
				"⚠️ Verification API error: %v\n",
				err,
			)

			select {
			case <-ctx.Done():
				return fmt.Errorf(
					"verification timed out while accessing Kubernetes API: %w",
					ctx.Err(),
				)

			case <-ticker.C:
				continue
			}
		}

		desired := int32(1)

		if deployment.Spec.Replicas != nil {
			desired = *deployment.Spec.Replicas
		}

		// Explicitly detect rollout failure.
		for _, condition := range deployment.Status.Conditions {

			if condition.Type ==
				appsv1.DeploymentProgressing {

				if condition.Reason ==
					"ProgressDeadlineExceeded" {

					return fmt.Errorf(
						"deployment rollout failed: %s",
						condition.Message,
					)
				}
			}
		}

		healthy :=
			deployment.Status.ObservedGeneration >=
				deployment.Generation &&

				deployment.Status.UpdatedReplicas ==
					desired &&

				deployment.Status.ReadyReplicas ==
					desired &&

				deployment.Status.AvailableReplicas ==
					desired &&

				deployment.Status.UnavailableReplicas ==
					0

		if healthy {
			return nil
		}

		fmt.Printf(
			"⏳ Deployment %s: ready %d/%d, updated %d/%d, available %d/%d\n",
			deploymentName,
			deployment.Status.ReadyReplicas,
			desired,
			deployment.Status.UpdatedReplicas,
			desired,
			deployment.Status.AvailableReplicas,
			desired,
		)

		select {

		case <-ctx.Done():
			return fmt.Errorf(
				"deployment did not become healthy before timeout: %w",
				ctx.Err(),
			)

		case <-ticker.C:
		}
	}
}

func IncreaseMemoryLimit(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) (string, string, string, error) {

	_, deployment, err := owningDeployment(ctx, clientset, pod)
	if err != nil {
		return "", "", "", err
	}

	containerName := ""

	if len(pod.Spec.Containers) > 0 {
		containerName = pod.Spec.Containers[0].Name
	}

	if containerName == "" {
		return "", "", "", fmt.Errorf(
			"pod %s/%s has no containers",
			pod.Namespace,
			pod.Name,
		)
	}

	max := resource.MustParse(MaxAutomaticMemoryLimit)

	for i := range deployment.Spec.Template.Spec.Containers {

		container := &deployment.Spec.Template.Spec.Containers[i]

		if container.Name != containerName {
			continue
		}

		current := container.Resources.Limits.Memory()

		if current.IsZero() {
			return "", "", "", fmt.Errorf(
				"container %s has no memory limit; automatic increase is unsafe",
				containerName,
			)
		}

		// HARD STOP.
		if current.Cmp(max) >= 0 {
			return "", "", "", fmt.Errorf(
				"memory limit already at maximum automatic remediation limit: %s",
				max.String(),
			)
		}

		currentBytes := current.Value()
		newBytes := currentBytes * 2

		// Never exceed the configured maximum.
		if newBytes > max.Value() {
			newBytes = max.Value()
		}

		newLimit := *resource.NewQuantity(
			newBytes,
			resource.BinarySI,
		)

		if newLimit.Cmp(*current) <= 0 {
			return "", "", "", fmt.Errorf(
				"calculated memory limit did not increase: %s -> %s",
				current.String(),
				newLimit.String(),
			)
		}

		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}

		container.Resources.Limits[corev1.ResourceMemory] = newLimit

		_, err = clientset.
			AppsV1().
			Deployments(deployment.Namespace).
			Update(
				ctx,
				deployment,
				metav1.UpdateOptions{},
			)

		if err != nil {
			return "", "", "", fmt.Errorf(
				"update deployment memory limit: %w",
				err,
			)
		}

		return deployment.Name, current.String(), newLimit.String(), nil
	}

	return "", "", "", fmt.Errorf(
		"container %s not found in deployment %s",
		containerName,
		deployment.Name,
	)
}

func RemediateOOM(
	ctx context.Context,
	clientset kubernetes.Interface,
	pod *corev1.Pod,
) (OOMRemediationResult, error) {

	result := OOMRemediationResult{}

	_, deployment, err := owningDeployment(
		ctx,
		clientset,
		pod,
	)

	if err != nil {
		return result, err
	}

	result.Deployment = deployment.Name

	for attempt := 1; attempt <= MaxAutomaticMemoryAttempts; attempt++ {

		result.Attempts = attempt

		fmt.Printf(
			"⚕️ OOM remediation attempt %d/%d\n",
			attempt,
			MaxAutomaticMemoryAttempts,
		)

		deploymentName, oldLimit, newLimit, err :=
			IncreaseMemoryLimit(
				ctx,
				clientset,
				pod,
			)

		if err != nil {

			result.LastMemoryLimit = oldLimit

			result.Exhausted = true
			result.Message = err.Error()

			return result, nil
		}

		result.LastMemoryLimit = newLimit

		fmt.Printf(
			"Memory: %s → %s\n",
			oldLimit,
			newLimit,
		)

		fmt.Println("⏳ Waiting for deployment recovery...")

		verifyCtx, cancel :=
			context.WithTimeout(
				ctx,
				45*time.Second,
			)

		err = WaitForDeploymentHealthy(
			verifyCtx,
			clientset,
			deployment.Namespace,
			deploymentName,
		)

		cancel()

		if err == nil {

			result.Resolved = true
			result.Message = fmt.Sprintf(
				"deployment became healthy after memory increase to %s",
				newLimit,
			)

			return result, nil
		}

		fmt.Printf(
			"⚠️ Deployment still unhealthy after %s: %v\n",
			newLimit,
			err,
		)
	}

	result.Exhausted = true
	result.Message = fmt.Sprintf(
		"automatic OOM remediation exhausted after %d attempts; maximum memory limit is %s",
		MaxAutomaticMemoryAttempts,
		MaxAutomaticMemoryLimit,
	)

	return result, nil
}
