package v1alpha1

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestDefaultsRetainPVC(t *testing.T) {
	agent := &HermesAgent{}
	agent.Default()

	if agent.Spec.Image != DefaultHermesImage {
		t.Fatalf("image = %q, want %q", agent.Spec.Image, DefaultHermesImage)
	}
	if agent.Spec.Persistence.Size == nil {
		t.Fatal("expected default storage size")
	}
	if got := agent.Spec.Persistence.Size.String(); got != DefaultStorageSize {
		t.Fatalf("storage size = %s, want %s", got, DefaultStorageSize)
	}
	if agent.Spec.Persistence.RetainOnDelete == nil || !*agent.Spec.Persistence.RetainOnDelete {
		t.Fatal("retainOnDelete should default to true")
	}
	if agent.Spec.Dashboard.Port != DefaultDashboardPort {
		t.Fatalf("dashboard port = %d, want %d", agent.Spec.Dashboard.Port, DefaultDashboardPort)
	}
	if agent.Spec.APIServer.Port != DefaultAPIServerPort {
		t.Fatalf("api server port = %d, want %d", agent.Spec.APIServer.Port, DefaultAPIServerPort)
	}
}

func TestValidateSpecDefaultsImage(t *testing.T) {
	agent := &HermesAgent{}
	agent.Default()

	if agent.Spec.Image != DefaultHermesImage {
		t.Fatalf("image = %q, want %q", agent.Spec.Image, DefaultHermesImage)
	}
	if errs := agent.ValidateSpec(); len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs.ToAggregate())
	}
}

func TestValidateSpecRequiresAPIServerKeySecretRef(t *testing.T) {
	agent := &HermesAgent{
		Spec: HermesAgentSpec{
			Image: "hermes-agent:test",
			APIServer: APIServerSpec{
				Enabled: true,
			},
		},
	}
	agent.Default()

	errs := agent.ValidateSpec()
	if len(errs) == 0 {
		t.Fatal("expected api server key secret validation error")
	}
	if !strings.Contains(errs.ToAggregate().Error(), "spec.apiServer.keySecretRef") {
		t.Fatalf("expected keySecretRef error, got %v", errs.ToAggregate())
	}
}

func TestValidateSpecAcceptsAPIServerKeySecretRef(t *testing.T) {
	agent := &HermesAgent{
		Spec: HermesAgentSpec{
			Image: "hermes-agent:test",
			APIServer: APIServerSpec{
				Enabled: true,
				KeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "hermes-secrets"},
					Key:                  "API_SERVER_KEY",
				},
			},
		},
	}
	agent.Default()

	if errs := agent.ValidateSpec(); len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs.ToAggregate())
	}
}

func TestSpecDoesNotExposePlaintextSecretFields(t *testing.T) {
	specType := reflect.TypeOf(HermesAgentSpec{})
	for i := 0; i < specType.NumField(); i++ {
		field := specType.Field(i)
		if field.Type.Kind() != reflect.String {
			continue
		}
		name := strings.ToLower(field.Name)
		jsonName := strings.ToLower(strings.Split(field.Tag.Get("json"), ",")[0])
		if strings.Contains(name, "secret") || strings.Contains(name, "token") || strings.Contains(name, "key") ||
			strings.Contains(jsonName, "secret") || strings.Contains(jsonName, "token") || strings.Contains(jsonName, "key") {
			t.Fatalf("plaintext-looking secret field found: %s", field.Name)
		}
	}
}
