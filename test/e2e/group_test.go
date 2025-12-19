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

var _ = Describe("KeycloakGroup", func() {
	const (
		groupName = "test-group"
		namespace = "default"
	)

	Context("When creating a KeycloakGroup", func() {
		It("Should create the group in Keycloak", func() {
			ctx := context.Background()

			// Create group
			group := &keycloakv1beta1.KeycloakGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      groupName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakGroupSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"name": "e2e-test-group"}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, group)).Should(Succeed())

			// Wait for group to become ready
			groupLookupKey := types.NamespacedName{Name: groupName, Namespace: namespace}
			createdGroup := &keycloakv1beta1.KeycloakGroup{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, groupLookupKey, createdGroup)
				if err != nil {
					return false
				}
				return createdGroup.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdGroup.Status.GroupID).ShouldNot(BeEmpty())

			// Cleanup
			Expect(k8sClient.Delete(ctx, group)).Should(Succeed())
		})
	})
})
