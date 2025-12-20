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

var _ = Describe("KeycloakProtocolMapper", func() {
	const (
		mapperName = "test-protocolmapper"
		namespace  = "default"
	)

	Context("When creating a KeycloakProtocolMapper", func() {
		It("Should create the mapper in Keycloak", func() {
			ctx := context.Background()

			clientRef := keycloakv1beta1.ResourceRef{Name: "test-client"}
			mapper := &keycloakv1beta1.KeycloakProtocolMapper{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mapperName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakProtocolMapperSpec{
					ClientRef: &clientRef,
					Definition: runtime.RawExtension{
						Raw: []byte(`{"name": "e2e-mapper", "protocol": "openid-connect", "protocolMapper": "oidc-usermodel-attribute-mapper"}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mapper)).Should(Succeed())

			mapperLookupKey := types.NamespacedName{Name: mapperName, Namespace: namespace}
			createdMapper := &keycloakv1beta1.KeycloakProtocolMapper{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, mapperLookupKey, createdMapper)
				if err != nil {
					return false
				}
				return createdMapper.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, mapper)).Should(Succeed())
		})
	})
})
