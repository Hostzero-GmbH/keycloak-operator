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

var _ = Describe("KeycloakRealm", func() {
	const (
		realmName = "test-realm"
		namespace = "default"
	)

	Context("When creating a KeycloakRealm", func() {
		It("Should create the realm in Keycloak", func() {
			ctx := context.Background()

			// Create realm
			realm := &keycloakv1beta1.KeycloakRealm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      realmName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakRealmSpec{
					InstanceRef: keycloakv1beta1.ResourceRef{
						Name: "test-instance",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"realm": "e2e-test-realm", "enabled": true}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, realm)).Should(Succeed())

			// Wait for realm to become ready
			realmLookupKey := types.NamespacedName{Name: realmName, Namespace: namespace}
			createdRealm := &keycloakv1beta1.KeycloakRealm{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, realmLookupKey, createdRealm)
				if err != nil {
					return false
				}
				return createdRealm.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdRealm.Status.ResourcePath).Should(ContainSubstring("e2e-test-realm"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, realm)).Should(Succeed())
		})
	})
})
