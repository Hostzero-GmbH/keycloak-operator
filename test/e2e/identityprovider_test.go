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

var _ = Describe("KeycloakIdentityProvider", func() {
	const (
		idpName   = "test-idp"
		namespace = "default"
	)

	Context("When creating a KeycloakIdentityProvider", func() {
		It("Should create the IdP in Keycloak", func() {
			ctx := context.Background()

			idp := &keycloakv1beta1.KeycloakIdentityProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      idpName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakIdentityProviderSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"alias": "e2e-google", "providerId": "google", "enabled": true}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, idp)).Should(Succeed())

			idpLookupKey := types.NamespacedName{Name: idpName, Namespace: namespace}
			createdIdp := &keycloakv1beta1.KeycloakIdentityProvider{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, idpLookupKey, createdIdp)
				if err != nil {
					return false
				}
				return createdIdp.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdIdp.Status.Alias).Should(Equal("e2e-google"))

			Expect(k8sClient.Delete(ctx, idp)).Should(Succeed())
		})
	})
})
