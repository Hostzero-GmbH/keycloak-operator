package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

var _ = Describe("Edge Cases", func() {
	Context("When instance reference is invalid", func() {
		It("Should report error status", func() {
			ctx := context.Background()

			realm := &keycloakv1beta1.KeycloakRealm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-instance-realm",
					Namespace: "default",
				},
				Spec: keycloakv1beta1.KeycloakRealmSpec{
					InstanceRef: keycloakv1beta1.ResourceRef{
						Name: "nonexistent-instance",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"realm": "test", "enabled": true}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, realm)).Should(Succeed())

			// Should not become ready
			Consistently(func() bool {
				created := &keycloakv1beta1.KeycloakRealm{}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(realm), created)
				if err != nil {
					return false
				}
				return created.Status.Ready
			}, "5s", interval).Should(BeFalse())

			Expect(k8sClient.Delete(ctx, realm)).Should(Succeed())
		})
	})

	Context("When definition is invalid JSON", func() {
		It("Should handle gracefully", func() {
			ctx := context.Background()

			realm := &keycloakv1beta1.KeycloakRealm{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-json-realm",
					Namespace: "default",
				},
				Spec: keycloakv1beta1.KeycloakRealmSpec{
					InstanceRef: keycloakv1beta1.ResourceRef{
						Name: "test-instance",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{invalid json}`),
					},
				},
			}
			// Should fail validation or report error status
			err := k8sClient.Create(ctx, realm)
			if err == nil {
				// Clean up if it was created
				_ = k8sClient.Delete(ctx, realm)
			}
		})
	})

	Context("When resource is deleted while being reconciled", func() {
		It("Should clean up finalizers", func() {
			ctx := context.Background()

			role := &keycloakv1beta1.KeycloakRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "quick-delete-role",
					Namespace: "default",
				},
				Spec: keycloakv1beta1.KeycloakRoleSpec{
					RealmRef: keycloakv1beta1.ResourceRef{
						Name: "test-realm",
					},
					Definition: runtime.RawExtension{
						Raw: []byte(`{"name": "quick-delete"}`),
					},
				},
			}
			Expect(k8sClient.Create(ctx, role)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, role)).Should(Succeed())

			// Should be fully deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(role), role)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})
	})
})
