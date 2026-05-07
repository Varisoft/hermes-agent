package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hermesv1alpha1 "github.com/nousresearch/hermes-agent/operators/hermes-operator/api/v1alpha1"
)

func TestReconcileCreatesCoreResources(t *testing.T) {
	ctx := context.Background()
	agent := testAgent()
	reconciler := testReconciler(t, agent)

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(agent)}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var pvc corev1.PersistentVolumeClaim
	if err := reconciler.Get(ctx, types.NamespacedName{Name: "coder-data", Namespace: "default"}, &pvc); err != nil {
		t.Fatalf("pvc not created: %v", err)
	}
	if len(pvc.OwnerReferences) != 0 {
		t.Fatalf("pvc should be retained by default, owner refs = %#v", pvc.OwnerReferences)
	}
	storage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if got := storage.String(); got != "10Gi" {
		t.Fatalf("pvc storage = %s, want 10Gi", got)
	}

	var cm corev1.ConfigMap
	if err := reconciler.Get(ctx, types.NamespacedName{Name: "coder-config", Namespace: "default"}, &cm); err != nil {
		t.Fatalf("configmap not created: %v", err)
	}
	if got := cm.Data["config.yaml"]; got == "" || got == "{}\n" {
		t.Fatalf("expected generated config.yaml, got %q", got)
	}
	if got := cm.Data["SOUL.md"]; got != "You are coder." {
		t.Fatalf("SOUL.md = %q", got)
	}

	var sts appsv1.StatefulSet
	if err := reconciler.Get(ctx, types.NamespacedName{Name: "coder", Namespace: "default"}, &sts); err != nil {
		t.Fatalf("statefulset not created: %v", err)
	}
	if got := sts.Spec.Template.Spec.Containers[0].Args; len(got) != 2 || got[0] != "gateway" || got[1] != "run" {
		t.Fatalf("container args = %#v, want gateway run", got)
	}
	if got := envValue(sts.Spec.Template.Spec.Containers[0].Env, "HERMES_DASHBOARD_HOST"); got != "0.0.0.0" {
		t.Fatalf("HERMES_DASHBOARD_HOST = %q", got)
	}
	if got := envValue(sts.Spec.Template.Spec.Containers[0].Env, "API_SERVER_HOST"); got != "0.0.0.0" {
		t.Fatalf("API_SERVER_HOST = %q", got)
	}
	if got := envValue(sts.Spec.Template.Spec.Containers[0].Env, "API_SERVER_MODEL_NAME"); got != "coder" {
		t.Fatalf("API_SERVER_MODEL_NAME = %q", got)
	}
	apiKeyEnv := findEnv(sts.Spec.Template.Spec.Containers[0].Env, "API_SERVER_KEY")
	if apiKeyEnv == nil || apiKeyEnv.ValueFrom == nil || apiKeyEnv.ValueFrom.SecretKeyRef == nil {
		t.Fatal("API_SERVER_KEY should come from a SecretKeyRef")
	}
	if apiKeyEnv.ValueFrom.SecretKeyRef.Name != "coder-hermes-secrets" || apiKeyEnv.ValueFrom.SecretKeyRef.Key != "API_SERVER_KEY" {
		t.Fatalf("API_SERVER_KEY secret ref = %#v", apiKeyEnv.ValueFrom.SecretKeyRef)
	}
	if len(sts.Spec.Template.Spec.Containers[0].EnvFrom) != 1 {
		t.Fatalf("envFrom count = %d, want 1", len(sts.Spec.Template.Spec.Containers[0].EnvFrom))
	}

	var svc corev1.Service
	if err := reconciler.Get(ctx, types.NamespacedName{Name: "coder", Namespace: "default"}, &svc); err != nil {
		t.Fatalf("service not created: %v", err)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("service type = %s, want ClusterIP", svc.Spec.Type)
	}
	if len(svc.Spec.Ports) != 2 {
		t.Fatalf("service ports = %#v, want dashboard and api", svc.Spec.Ports)
	}
}

func TestReconcileDoesNotCreateServiceWhenNoSurfaceEnabled(t *testing.T) {
	ctx := context.Background()
	agent := &hermesv1alpha1.HermesAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "default"},
		Spec: hermesv1alpha1.HermesAgentSpec{
			Image: "hermes-agent:test",
		},
	}
	reconciler := testReconciler(t, agent)

	if _, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(agent)}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var svc corev1.Service
	err := reconciler.Get(ctx, types.NamespacedName{Name: "worker", Namespace: "default"}, &svc)
	if err == nil {
		t.Fatal("service should not be created when dashboard and api server are disabled")
	}
}

func testReconciler(t *testing.T, objects ...client.Object) *HermesAgentReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add core scheme: %v", err)
	}
	if err := hermesv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add hermes scheme: %v", err)
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
	return &HermesAgentReconciler{Client: cl, Scheme: scheme}
}

func testAgent() *hermesv1alpha1.HermesAgent {
	return &hermesv1alpha1.HermesAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "coder", Namespace: "default"},
		Spec: hermesv1alpha1.HermesAgentSpec{
			Image:  "hermes-agent:test",
			Config: &apiextensionsv1.JSON{Raw: []byte(`{"model":{"provider":"openai","default":"gpt-4.1"}}`)},
			Soul:   "You are coder.",
			EnvFromSecrets: []corev1.LocalObjectReference{
				{Name: "coder-hermes-secrets"},
			},
			Dashboard: hermesv1alpha1.DashboardSpec{
				Enabled: true,
				Port:    9119,
			},
			APIServer: hermesv1alpha1.APIServerSpec{
				Enabled: true,
				Port:    8642,
				KeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "coder-hermes-secrets"},
					Key:                  "API_SERVER_KEY",
				},
			},
		},
	}
}

func envValue(env []corev1.EnvVar, name string) string {
	if found := findEnv(env, name); found != nil {
		return found.Value
	}
	return ""
}

func findEnv(env []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range env {
		if env[i].Name == name {
			return &env[i]
		}
	}
	return nil
}
