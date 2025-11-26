package v1alpha1

import (
    "k8s.io/apimachinery/pkg/runtime/schema"
    "sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is the API group and version used to register these objects.
var GroupVersion = schema.GroupVersion{
    Group:   "autoscaler.parspack.dev",
    Version: "v1alpha1",
}

// SchemeBuilder is used by controller-runtime to add Go types to the global scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme adds all types of this group-version to a Scheme.
var AddToScheme = SchemeBuilder.AddToScheme
