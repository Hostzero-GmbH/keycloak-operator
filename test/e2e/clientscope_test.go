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

var _ = Describe("KeycloakClientScope", func() {
	const (
		scopeName = "test-clientscope"
		namespace = "default"
	)

	Context("When creating a KeycloakClientScope", func() {
		It("Should create the client scope in Keycloak", func() {
			ctx := context.Background()

			scope := &keycloakv1beta1.KeycloakClientScope{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scopeName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakClientScopeSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"name": "e2e-test-scope", "protocol": "openid-connect"}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, scope)).Should(Succeed())

			scopeLookupKey := types.NamespacedName{Name: scopeName, Namespace: namespace}
			createdScope := &keycloakv1beta1.KeycloakClientScope{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, scopeLookupKey, createdScope)
				if err != nil {
					return false
				}
				return createdScope.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdScope.Status.ScopeID).ShouldNot(BeEmpty())

			Expect(k8sClient.Delete(ctx, scope)).Should(Succeed())
		})
	})
})
