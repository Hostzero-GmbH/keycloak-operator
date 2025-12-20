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

var _ = Describe("KeycloakComponent", func() {
	const (
		componentName = "test-component"
		namespace     = "default"
	)

	Context("When creating a KeycloakComponent", func() {
		It("Should create the component in Keycloak", func() {
			ctx := context.Background()

			component := &keycloakv1beta1.KeycloakComponent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      componentName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakComponentSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"name": "e2e-component", "providerType": "org.keycloak.keys.KeyProvider", "providerId": "rsa-generated"}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, component)).Should(Succeed())

			componentLookupKey := types.NamespacedName{Name: componentName, Namespace: namespace}
			createdComponent := &keycloakv1beta1.KeycloakComponent{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, componentLookupKey, createdComponent)
				if err != nil {
					return false
				}
				return createdComponent.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdComponent.Status.ComponentID).ShouldNot(BeEmpty())

			Expect(k8sClient.Delete(ctx, component)).Should(Succeed())
		})
	})
})
