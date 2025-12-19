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

var _ = Describe("KeycloakUser", func() {
	const (
		userName  = "test-user"
		namespace = "default"
	)

	Context("When creating a KeycloakUser", func() {
		It("Should create the user in Keycloak", func() {
			ctx := context.Background()

			// Create user
			user := &keycloakv1beta1.KeycloakUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakUserSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"username": "e2e-test-user", "enabled": true, "email": "test@example.com"}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, user)).Should(Succeed())

			// Wait for user to become ready
			userLookupKey := types.NamespacedName{Name: userName, Namespace: namespace}
			createdUser := &keycloakv1beta1.KeycloakUser{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, userLookupKey, createdUser)
				if err != nil {
					return false
				}
				return createdUser.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdUser.Status.UserID).ShouldNot(BeEmpty())

			// Cleanup
			Expect(k8sClient.Delete(ctx, user)).Should(Succeed())
		})
	})
})
