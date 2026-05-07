package v1alpha1

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	DefaultHermesImage   = "nousresearch/hermes-agent:0.12.0"
	DefaultStorageSize   = "10Gi"
	DefaultDashboardPort = int32(9119)
	DefaultAPIServerPort = int32(8642)
)

// HermesAgentSpec defines the desired state of HermesAgent.
type HermesAgentSpec struct {
	// Image is the Hermes container image to run.
	// +optional
	// +kubebuilder:default:="nousresearch/hermes-agent:0.12.0"
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image,omitempty"`

	// ImagePullPolicy controls when Kubernetes pulls the Hermes image.
	// +kubebuilder:validation:Enum=Always;Never;IfNotPresent
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`

	// Persistence configures the PVC mounted as HERMES_HOME at /opt/data.
	// +optional
	Persistence PersistenceSpec `json:"persistence,omitempty"`

	// Config is written as the initial config.yaml when the data PVC does not
	// already contain one. It intentionally preserves Hermes config keys without
	// the operator trying to mirror the Python config schema.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Config *apiextensionsv1.JSON `json:"config,omitempty"`

	// Soul is written as the initial SOUL.md when the data PVC does not already
	// contain one.
	// +optional
	Soul string `json:"soul,omitempty"`

	// EnvFromSecrets references Kubernetes Secrets to expose as environment
	// variables. The CRD has no plaintext secret fields.
	// +optional
	EnvFromSecrets []corev1.LocalObjectReference `json:"envFromSecrets,omitempty"`

	// Dashboard controls the optional in-container dashboard side process.
	// +optional
	Dashboard DashboardSpec `json:"dashboard,omitempty"`

	// APIServer controls the optional OpenAI-compatible API server exposed by the
	// Hermes gateway.
	// +optional
	APIServer APIServerSpec `json:"apiServer,omitempty"`

	// Resources configures CPU and memory requests/limits for the Hermes pod.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// PersistenceSpec configures Hermes state storage.
type PersistenceSpec struct {
	// Size is the requested PVC size.
	// +optional
	// +kubebuilder:default:="10Gi"
	Size *resource.Quantity `json:"size,omitempty"`

	// RetainOnDelete keeps the PVC after the HermesAgent CR is deleted.
	// +optional
	// +kubebuilder:default:=true
	RetainOnDelete *bool `json:"retainOnDelete,omitempty"`
}

// DashboardSpec configures the Hermes dashboard side process.
type DashboardSpec struct {
	// Enabled starts `hermes dashboard` as a side process in the Hermes
	// container entrypoint.
	// +optional
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled,omitempty"`

	// TUI enables the embedded browser chat tab.
	// +optional
	// +kubebuilder:default:=false
	TUI bool `json:"tui,omitempty"`

	// Port is the dashboard listen port.
	// +optional
	// +kubebuilder:default:=9119
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`
}

// APIServerSpec configures the OpenAI-compatible API server.
// +kubebuilder:validation:XValidation:rule="!has(self.enabled) || self.enabled == false || (has(self.keySecretRef) && has(self.keySecretRef.name) && size(self.keySecretRef.name) > 0 && has(self.keySecretRef.key) && size(self.keySecretRef.key) > 0)",message="apiServer.keySecretRef.name and apiServer.keySecretRef.key are required when apiServer.enabled is true"
type APIServerSpec struct {
	// Enabled starts the API server inside `hermes gateway run`.
	// +optional
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled,omitempty"`

	// Port is the API server listen port.
	// +optional
	// +kubebuilder:default:=8642
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port,omitempty"`

	// KeySecretRef points to the API_SERVER_KEY value. Required when enabled.
	// +optional
	KeySecretRef *corev1.SecretKeySelector `json:"keySecretRef,omitempty"`
}

// HermesAgentStatus defines the observed state of HermesAgent.
type HermesAgentStatus struct {
	// Phase is a short machine-readable summary of the current lifecycle state.
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions reports readiness and validation details.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation reconciled by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// StatefulSetName is the managed workload name.
	// +optional
	StatefulSetName string `json:"statefulSetName,omitempty"`

	// PVCName is the managed data PVC name.
	// +optional
	PVCName string `json:"pvcName,omitempty"`

	// ServiceName is the managed ClusterIP service name, when any port is enabled.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// ReadyReplicas is copied from the managed StatefulSet status.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// InternalURLs lists in-cluster URLs for enabled HTTP surfaces.
	// +optional
	InternalURLs []string `json:"internalURLs,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ha
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyReplicas`
// +kubebuilder:printcolumn:name="Image",type=string,JSONPath=`.spec.image`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type HermesAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HermesAgentSpec   `json:"spec,omitempty"`
	Status HermesAgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type HermesAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HermesAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HermesAgent{}, &HermesAgentList{})
}

func (in *HermesAgent) Default() {
	if strings.TrimSpace(in.Spec.Image) == "" {
		in.Spec.Image = DefaultHermesImage
	}
	if in.Spec.Persistence.Size == nil {
		q := resource.MustParse(DefaultStorageSize)
		in.Spec.Persistence.Size = &q
	}
	if in.Spec.Persistence.RetainOnDelete == nil {
		v := true
		in.Spec.Persistence.RetainOnDelete = &v
	}
	if in.Spec.Dashboard.Port == 0 {
		in.Spec.Dashboard.Port = DefaultDashboardPort
	}
	if in.Spec.APIServer.Port == 0 {
		in.Spec.APIServer.Port = DefaultAPIServerPort
	}
}

func (in *HermesAgent) ValidateSpec() field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if in.Spec.Dashboard.Port < 0 || in.Spec.Dashboard.Port > 65535 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("dashboard", "port"), in.Spec.Dashboard.Port, "must be between 1 and 65535"))
	}
	if in.Spec.APIServer.Port < 0 || in.Spec.APIServer.Port > 65535 {
		allErrs = append(allErrs, field.Invalid(specPath.Child("apiServer", "port"), in.Spec.APIServer.Port, "must be between 1 and 65535"))
	}

	if in.Spec.APIServer.Enabled {
		if in.Spec.APIServer.KeySecretRef == nil {
			allErrs = append(allErrs, field.Required(specPath.Child("apiServer", "keySecretRef"), "keySecretRef is required when apiServer.enabled is true"))
		} else {
			if strings.TrimSpace(in.Spec.APIServer.KeySecretRef.Name) == "" {
				allErrs = append(allErrs, field.Required(specPath.Child("apiServer", "keySecretRef", "name"), "secret name is required"))
			}
			if strings.TrimSpace(in.Spec.APIServer.KeySecretRef.Key) == "" {
				allErrs = append(allErrs, field.Required(specPath.Child("apiServer", "keySecretRef", "key"), "secret key is required"))
			}
		}
	}

	return allErrs
}

func (in *HermesAgent) ValidateSpecError() error {
	if errs := in.ValidateSpec(); len(errs) > 0 {
		return fmt.Errorf(errs.ToAggregate().Error())
	}
	return nil
}

func (in *HermesAgent) RetainPVC() bool {
	if in.Spec.Persistence.RetainOnDelete == nil {
		return true
	}
	return *in.Spec.Persistence.RetainOnDelete
}

func (in *HermesAgent) StorageSize() resource.Quantity {
	if in.Spec.Persistence.Size == nil {
		return resource.MustParse(DefaultStorageSize)
	}
	return *in.Spec.Persistence.Size
}
