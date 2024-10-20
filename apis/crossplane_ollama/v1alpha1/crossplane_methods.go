package v1alpha1

/*
generating these methods via angryjet failed for me, error msg wasn't helpful
TODO: use angryjet in the future, for now implementing those methods myself is easy enough
*/

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

var _ resource.Managed = &Model{}

func (in *Model) GetProviderConfigReference() *xpv1.Reference {
	return in.Spec.ProviderConfigReference
}

func (in *Model) SetProviderConfigReference(p *xpv1.Reference) {
	in.Spec.ProviderConfigReference = p
}

func (in *Model) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	in.Spec.WriteConnectionSecretToReference = r
}

func (in *Model) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return in.Spec.WriteConnectionSecretToReference
}

func (in *Model) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	in.Spec.PublishConnectionDetailsTo = r
}

func (in *Model) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return in.Spec.PublishConnectionDetailsTo
}

func (in *Model) SetManagementPolicies(p xpv1.ManagementPolicies) {
	in.Spec.ManagementPolicies = p
}

func (in *Model) GetManagementPolicies() xpv1.ManagementPolicies {
	return in.Spec.ManagementPolicies
}

func (in *Model) SetDeletionPolicy(dp xpv1.DeletionPolicy) {
	in.Spec.DeletionPolicy = dp
}

func (in *Model) GetDeletionPolicy() xpv1.DeletionPolicy {
	return in.Spec.DeletionPolicy
}

func (in *Model) SetConditions(c ...xpv1.Condition) {
	in.Status.SetConditions(c...)
}

func (in *Model) SetConditionsWithObservedGeneration(c ...xpv1.Condition) {
	for i := range c {
		c[i].ObservedGeneration = in.Generation
	}

	in.Status.SetConditions(c...)
}

func (in *Model) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return in.Status.GetCondition(ct)
}

var _ resource.ProviderConfigUsage = &ProviderConfigUsage{}

func (in *ProviderConfigUsage) GetProviderConfigReference() xpv1.Reference {
	return in.ProviderConfigReference
}

func (in *ProviderConfigUsage) SetProviderConfigReference(p xpv1.Reference) {
	in.ProviderConfigReference = p
}

func (in *ProviderConfigUsage) SetResourceReference(r xpv1.TypedReference) {
	in.ResourceReference = r
}

func (in *ProviderConfigUsage) GetResourceReference() xpv1.TypedReference {
	return in.ResourceReference
}
