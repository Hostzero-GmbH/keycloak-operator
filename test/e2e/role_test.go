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

var _ = Describe("KeycloakRole", func() {
	const (
		roleName  = "test-role"
		namespace = "default"
	)

	Context("When creating a KeycloakRole", func() {
		It("Should create the role in Keycloak", func() {
			ctx := context.Background()

			// Create role
			role := &keycloakv1beta1.KeycloakRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roleName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakRoleSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"name": "e2e-test-role", "description": "Test role"}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, role)).Should(Succeed())

			// Wait for role to become ready
			roleLookupKey := types.NamespacedName{Name: roleName, Namespace: namespace}
			createdRole := &keycloakv1beta1.KeycloakRole{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, roleLookupKey, createdRole)
				if err != nil {
					return false
				}
				return createdRole.Status.Ready
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, role)).Should(Succeed())
		})
	})
})
