package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

var _ = Describe("KeycloakOrganization", func() {
	const (
		orgName   = "test-organization"
		namespace = "default"
	)

	Context("When creating a KeycloakOrganization", func() {
		It("Should create the organization in Keycloak 26+", func() {
			ctx := context.Background()

			org := &keycloakv1beta1.KeycloakOrganization{
				ObjectMeta: metav1.ObjectMeta{
					Name:      orgName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakOrganizationSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"name": "e2e-test-org", "enabled": true}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, org)).Should(Succeed())

			orgLookupKey := types.NamespacedName{Name: orgName, Namespace: namespace}
			createdOrg := &keycloakv1beta1.KeycloakOrganization{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, orgLookupKey, createdOrg)
				if err != nil {
					return false
				}
				return createdOrg.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdOrg.Status.OrganizationID).ShouldNot(BeEmpty())

			Expect(k8sClient.Delete(ctx, org)).Should(Succeed())
		})
	})
})
