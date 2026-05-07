package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	hermesv1alpha1 "github.com/nousresearch/hermes-agent/operators/hermes-operator/api/v1alpha1"
)

const (
	conditionReady = "Ready"

	phasePending = "Pending"
	phaseReady   = "Ready"
	phaseInvalid = "Invalid"

	configMountName = "hermes-config"
	dataMountName   = "hermes-data"
)

// HermesAgentReconciler reconciles a HermesAgent object.
type HermesAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=hermes.nousresearch.com,resources=hermesagents,verbs=get;list;watch
// +kubebuilder:rbac:groups=hermes.nousresearch.com,resources=hermesagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=configmaps;services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete

func (r *HermesAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var agent hermesv1alpha1.HermesAgent
	if err := r.Get(ctx, req.NamespacedName, &agent); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	agent.Default()

	if err := agent.ValidateSpecError(); err != nil {
		logger.Error(err, "invalid HermesAgent spec")
		return ctrl.Result{}, r.updateStatus(ctx, &agent, phaseInvalid, 0, []string{}, metav1.Condition{
			Type:               conditionReady,
			Status:             metav1.ConditionFalse,
			Reason:             "InvalidSpec",
			Message:            err.Error(),
			ObservedGeneration: agent.Generation,
		})
	}

	if err := r.reconcilePVC(ctx, &agent); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileConfigMap(ctx, &agent); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileStatefulSet(ctx, &agent); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.reconcileService(ctx, &agent); err != nil {
		return ctrl.Result{}, err
	}

	readyReplicas := int32(0)
	var sts appsv1.StatefulSet
	if err := r.Get(ctx, types.NamespacedName{Name: workloadName(&agent), Namespace: agent.Namespace}, &sts); err == nil {
		readyReplicas = sts.Status.ReadyReplicas
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	phase := phasePending
	conditionStatus := metav1.ConditionFalse
	conditionReason := "WaitingForStatefulSet"
	conditionMessage := "Hermes StatefulSet is not ready"
	if readyReplicas > 0 {
		phase = phaseReady
		conditionStatus = metav1.ConditionTrue
		conditionReason = "Ready"
		conditionMessage = "Hermes StatefulSet has ready replicas"
	}

	return ctrl.Result{}, r.updateStatus(ctx, &agent, phase, readyReplicas, internalURLs(&agent), metav1.Condition{
		Type:               conditionReady,
		Status:             conditionStatus,
		Reason:             conditionReason,
		Message:            conditionMessage,
		ObservedGeneration: agent.Generation,
	})
}

func (r *HermesAgentReconciler) reconcilePVC(ctx context.Context, agent *hermesv1alpha1.HermesAgent) error {
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = pvcName(agent)
	pvc.Namespace = agent.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		labels := labelsFor(agent)
		pvc.Labels = labels
		pvc.Spec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		if pvc.Spec.Resources.Requests == nil {
			pvc.Spec.Resources.Requests = corev1.ResourceList{}
		}
		desiredSize := agent.StorageSize()
		currentSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		if currentSize.IsZero() || desiredSize.Cmp(currentSize) > 0 {
			pvc.Spec.Resources.Requests[corev1.ResourceStorage] = desiredSize
		}

		if agent.RetainPVC() {
			pvc.OwnerReferences = removeOwnerRef(pvc.OwnerReferences, agent)
			return nil
		}
		return controllerutil.SetControllerReference(agent, pvc, r.Scheme)
	})
	return err
}

func (r *HermesAgentReconciler) reconcileConfigMap(ctx context.Context, agent *hermesv1alpha1.HermesAgent) error {
	cm := &corev1.ConfigMap{}
	cm.Name = configMapName(agent)
	cm.Namespace = agent.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Labels = labelsFor(agent)
		cm.Data = map[string]string{
			"config.yaml": configYAML(agent),
		}
		if strings.TrimSpace(agent.Spec.Soul) != "" {
			cm.Data["SOUL.md"] = agent.Spec.Soul
		}
		return controllerutil.SetControllerReference(agent, cm, r.Scheme)
	})
	return err
}

func (r *HermesAgentReconciler) reconcileStatefulSet(ctx context.Context, agent *hermesv1alpha1.HermesAgent) error {
	sts := &appsv1.StatefulSet{}
	sts.Name = workloadName(agent)
	sts.Namespace = agent.Namespace

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		labels := labelsFor(agent)
		replicas := int32(1)
		sts.Labels = labels
		sts.Spec.ServiceName = serviceName(agent)
		sts.Spec.Replicas = &replicas
		sts.Spec.Selector = &metav1.LabelSelector{MatchLabels: selectorLabelsFor(agent)}
		sts.Spec.Template.ObjectMeta.Labels = labels
		sts.Spec.Template.Spec.InitContainers = []corev1.Container{initConfigContainer(agent)}
		sts.Spec.Template.Spec.Containers = []corev1.Container{hermesContainer(agent)}
		sts.Spec.Template.Spec.Volumes = volumesFor(agent)
		return controllerutil.SetControllerReference(agent, sts, r.Scheme)
	})
	return err
}

func (r *HermesAgentReconciler) reconcileService(ctx context.Context, agent *hermesv1alpha1.HermesAgent) error {
	ports := servicePorts(agent)
	key := types.NamespacedName{Name: serviceName(agent), Namespace: agent.Namespace}

	if len(ports) == 0 {
		svc := &corev1.Service{}
		err := r.Get(ctx, key, svc)
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return r.Delete(ctx, svc)
	}

	svc := &corev1.Service{}
	svc.Name = key.Name
	svc.Namespace = key.Namespace
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Labels = labelsFor(agent)
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Selector = selectorLabelsFor(agent)
		svc.Spec.Ports = ports
		return controllerutil.SetControllerReference(agent, svc, r.Scheme)
	})
	return err
}

func (r *HermesAgentReconciler) updateStatus(ctx context.Context, agent *hermesv1alpha1.HermesAgent, phase string, readyReplicas int32, urls []string, ready metav1.Condition) error {
	latest := &hermesv1alpha1.HermesAgent{}
	if err := r.Get(ctx, types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}, latest); err != nil {
		return err
	}

	latest.Status.Phase = phase
	latest.Status.ObservedGeneration = latest.Generation
	latest.Status.StatefulSetName = workloadName(latest)
	latest.Status.PVCName = pvcName(latest)
	if len(servicePorts(latest)) > 0 {
		latest.Status.ServiceName = serviceName(latest)
	} else {
		latest.Status.ServiceName = ""
	}
	latest.Status.ReadyReplicas = readyReplicas
	latest.Status.InternalURLs = urls
	apiMeta.SetStatusCondition(&latest.Status.Conditions, ready)

	if err := r.Status().Update(ctx, latest); err != nil {
		return err
	}
	return nil
}

func initConfigContainer(agent *hermesv1alpha1.HermesAgent) corev1.Container {
	return corev1.Container{
		Name:            "init-hermes-home",
		Image:           agent.Spec.Image,
		ImagePullPolicy: imagePullPolicy(agent),
		Command: []string{
			"/bin/sh",
			"-c",
			`set -eu
if [ -f /config/config.yaml ] && [ ! -f /data/config.yaml ]; then cp /config/config.yaml /data/config.yaml; fi
if [ -f /config/SOUL.md ] && [ ! -f /data/SOUL.md ]; then cp /config/SOUL.md /data/SOUL.md; fi`,
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: dataMountName, MountPath: "/data"},
			{Name: configMountName, MountPath: "/config", ReadOnly: true},
		},
	}
}

func hermesContainer(agent *hermesv1alpha1.HermesAgent) corev1.Container {
	container := corev1.Container{
		Name:            "hermes",
		Image:           agent.Spec.Image,
		ImagePullPolicy: imagePullPolicy(agent),
		Args:            []string{"gateway", "run"},
		Env:             envFor(agent),
		EnvFrom:         envFromFor(agent),
		Ports:           containerPorts(agent),
		Resources:       agent.Spec.Resources,
		VolumeMounts: []corev1.VolumeMount{
			{Name: dataMountName, MountPath: "/opt/data"},
		},
	}
	return container
}

func envFor(agent *hermesv1alpha1.HermesAgent) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "HERMES_HOME", Value: "/opt/data"},
	}

	if agent.Spec.Dashboard.Enabled {
		env = append(env,
			corev1.EnvVar{Name: "HERMES_DASHBOARD", Value: "1"},
			corev1.EnvVar{Name: "HERMES_DASHBOARD_HOST", Value: "0.0.0.0"},
			corev1.EnvVar{Name: "HERMES_DASHBOARD_PORT", Value: fmt.Sprintf("%d", dashboardPort(agent))},
		)
		if agent.Spec.Dashboard.TUI {
			env = append(env, corev1.EnvVar{Name: "HERMES_DASHBOARD_TUI", Value: "1"})
		}
	}

	if agent.Spec.APIServer.Enabled {
		env = append(env,
			corev1.EnvVar{Name: "API_SERVER_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "API_SERVER_HOST", Value: "0.0.0.0"},
			corev1.EnvVar{Name: "API_SERVER_PORT", Value: fmt.Sprintf("%d", apiServerPort(agent))},
			corev1.EnvVar{Name: "API_SERVER_MODEL_NAME", Value: agent.Name},
			corev1.EnvVar{
				Name: "API_SERVER_KEY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: agent.Spec.APIServer.KeySecretRef,
				},
			},
		)
	}

	return env
}

func envFromFor(agent *hermesv1alpha1.HermesAgent) []corev1.EnvFromSource {
	out := make([]corev1.EnvFromSource, 0, len(agent.Spec.EnvFromSecrets))
	for _, ref := range agent.Spec.EnvFromSecrets {
		out = append(out, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: ref,
			},
		})
	}
	return out
}

func containerPorts(agent *hermesv1alpha1.HermesAgent) []corev1.ContainerPort {
	var ports []corev1.ContainerPort
	if agent.Spec.Dashboard.Enabled {
		ports = append(ports, corev1.ContainerPort{Name: "dashboard", ContainerPort: dashboardPort(agent)})
	}
	if agent.Spec.APIServer.Enabled {
		ports = append(ports, corev1.ContainerPort{Name: "api", ContainerPort: apiServerPort(agent)})
	}
	return ports
}

func servicePorts(agent *hermesv1alpha1.HermesAgent) []corev1.ServicePort {
	var ports []corev1.ServicePort
	if agent.Spec.Dashboard.Enabled {
		ports = append(ports, corev1.ServicePort{Name: "dashboard", Port: dashboardPort(agent), TargetPort: intstrFromString("dashboard")})
	}
	if agent.Spec.APIServer.Enabled {
		ports = append(ports, corev1.ServicePort{Name: "api", Port: apiServerPort(agent), TargetPort: intstrFromString("api")})
	}
	return ports
}

func volumesFor(agent *hermesv1alpha1.HermesAgent) []corev1.Volume {
	return []corev1.Volume{
		{
			Name: dataMountName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName(agent)},
			},
		},
		{
			Name: configMountName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: configMapName(agent)},
				},
			},
		},
	}
}

func configYAML(agent *hermesv1alpha1.HermesAgent) string {
	if agent.Spec.Config == nil || len(agent.Spec.Config.Raw) == 0 {
		return "{}\n"
	}
	out, err := yaml.JSONToYAML(agent.Spec.Config.Raw)
	if err != nil {
		return "{}\n"
	}
	return string(out)
}

func internalURLs(agent *hermesv1alpha1.HermesAgent) []string {
	host := fmt.Sprintf("%s.%s.svc", serviceName(agent), agent.Namespace)
	var urls []string
	if agent.Spec.Dashboard.Enabled {
		urls = append(urls, fmt.Sprintf("http://%s:%d", host, dashboardPort(agent)))
	}
	if agent.Spec.APIServer.Enabled {
		urls = append(urls, fmt.Sprintf("http://%s:%d/v1", host, apiServerPort(agent)))
	}
	return urls
}

func imagePullPolicy(agent *hermesv1alpha1.HermesAgent) corev1.PullPolicy {
	if agent.Spec.ImagePullPolicy == "" {
		return corev1.PullIfNotPresent
	}
	return agent.Spec.ImagePullPolicy
}

func dashboardPort(agent *hermesv1alpha1.HermesAgent) int32 {
	if agent.Spec.Dashboard.Port == 0 {
		return hermesv1alpha1.DefaultDashboardPort
	}
	return agent.Spec.Dashboard.Port
}

func apiServerPort(agent *hermesv1alpha1.HermesAgent) int32 {
	if agent.Spec.APIServer.Port == 0 {
		return hermesv1alpha1.DefaultAPIServerPort
	}
	return agent.Spec.APIServer.Port
}

func workloadName(agent *hermesv1alpha1.HermesAgent) string {
	return agent.Name
}

func pvcName(agent *hermesv1alpha1.HermesAgent) string {
	return agent.Name + "-data"
}

func configMapName(agent *hermesv1alpha1.HermesAgent) string {
	return agent.Name + "-config"
}

func serviceName(agent *hermesv1alpha1.HermesAgent) string {
	return agent.Name
}

func labelsFor(agent *hermesv1alpha1.HermesAgent) map[string]string {
	labels := selectorLabelsFor(agent)
	labels["app.kubernetes.io/managed-by"] = "hermes-operator"
	labels["app.kubernetes.io/part-of"] = "hermes"
	return labels
}

func selectorLabelsFor(agent *hermesv1alpha1.HermesAgent) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "hermes-agent",
		"app.kubernetes.io/instance": agent.Name,
	}
}

func removeOwnerRef(refs []metav1.OwnerReference, agent *hermesv1alpha1.HermesAgent) []metav1.OwnerReference {
	out := refs[:0]
	for _, ref := range refs {
		if ref.UID == agent.UID && ref.Kind == "HermesAgent" {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func intstrFromString(value string) intstr.IntOrString {
	return intstr.FromString(value)
}

// SetupWithManager sets up the controller with the Manager.
func (r *HermesAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hermesv1alpha1.HermesAgent{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
