// Package v1alpha1 contains API Schema definitions for the hermes v1alpha1 API group.
// +kubebuilder:object:generate=true
// +groupName=hermes.nousresearch.com
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion = schema.GroupVersion{Group: "hermes.nousresearch.com", Version: "v1alpha1"}

	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)
