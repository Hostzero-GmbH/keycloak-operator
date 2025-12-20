package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

var _ = Describe("KeycloakRoleMapping", func() {
	const (
		mappingName = "test-rolemapping"
		namespace   = "default"
	)

	Context("When creating a KeycloakRoleMapping", func() {
		It("Should assign roles to the user", func() {
			ctx := context.Background()

			mapping := &keycloakv1beta1.KeycloakRoleMapping{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mappingName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakRoleMappingSpec{
					UserRef: keycloakv1beta1.ResourceRef{
						Name: "test-user",
					},
					RealmRoles: []string{"e2e-test-role"},
				},
			}
			Expect(k8sClient.Create(ctx, mapping)).Should(Succeed())

			mappingLookupKey := types.NamespacedName{Name: mappingName, Namespace: namespace}
			createdMapping := &keycloakv1beta1.KeycloakRoleMapping{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, mappingLookupKey, createdMapping)
				if err != nil {
					return false
				}
				return createdMapping.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, mapping)).Should(Succeed())
		})
	})
})
