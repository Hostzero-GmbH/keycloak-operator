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

var _ = Describe("KeycloakClient", func() {
	const (
		clientName = "test-client"
		namespace  = "default"
	)

	Context("When creating a KeycloakClient", func() {
		It("Should create the client in Keycloak", func() {
			ctx := context.Background()

			// Create client
			kcClient := &keycloakv1beta1.KeycloakClient{
				ObjectMeta: metav1.ObjectMeta{
					Name:      clientName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakClientSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"clientId": "e2e-test-client", "enabled": true, "publicClient": true}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, kcClient)).Should(Succeed())

			// Wait for client to become ready
			clientLookupKey := types.NamespacedName{Name: clientName, Namespace: namespace}
			createdClient := &keycloakv1beta1.KeycloakClient{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, clientLookupKey, createdClient)
				if err != nil {
					return false
				}
				return createdClient.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdClient.Status.ClientUUID).ShouldNot(BeEmpty())

			// Cleanup
			Expect(k8sClient.Delete(ctx, kcClient)).Should(Succeed())
		})
	})
})
