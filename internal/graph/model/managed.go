package model

import (
	kunstructured "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"

	"github.com/upbound/xgql/internal/unstructured"
)

// A ManagedResourceSpec specifies the desired state of a managed resource.
type ManagedResourceSpec struct {
	WritesConnectionSecretToRef *xpv1.SecretReference
	ProviderConfigRef           *xpv1.Reference
	DeletionPolicy              *DeletionPolicy `json:"deletionPolicy"`
}

// GetDeletionPolicy from the supplied Crossplane policy.
func GetDeletionPolicy(p xpv1.DeletionPolicy) *DeletionPolicy {
	out := DeletionPolicy(p)
	return &out
}

// GetManagedResource from the supplied Crossplane resource.
func GetManagedResource(u *kunstructured.Unstructured) ManagedResource {
	mg := &unstructured.Managed{Unstructured: *u}
	return ManagedResource{
		ID: ReferenceID{
			APIVersion: mg.GetAPIVersion(),
			Kind:       mg.GetKind(),
			Name:       mg.GetName(),
		},

		APIVersion: mg.GetAPIVersion(),
		Kind:       mg.GetKind(),
		Metadata:   GetObjectMeta(mg),
		Spec: &ManagedResourceSpec{
			WritesConnectionSecretToRef: mg.GetWriteConnectionSecretToReference(),
			ProviderConfigRef:           mg.GetProviderConfigReference(),
			DeletionPolicy:              GetDeletionPolicy(mg.GetDeletionPolicy()),
		},
		Status: &ManagedResourceStatus{
			Conditions: GetConditions(mg.GetConditions()),
		},
		Raw: raw(mg),
	}
}
