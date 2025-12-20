package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

var _ = Describe("KeycloakUserCredential", func() {
	const (
		credName  = "test-usercredential"
		namespace = "default"
	)

	Context("When creating a KeycloakUserCredential", func() {
		It("Should set the user password", func() {
			ctx := context.Background()

			// Create password secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user-password",
					Namespace: namespace,
				},
				StringData: map[string]string{
					"password": "testpassword123",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			cred := &keycloakv1beta1.KeycloakUserCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      credName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakUserCredentialSpec{
					UserRef: keycloakv1beta1.ResourceRef{
						Name: "test-user",
					},
					SecretRef: keycloakv1beta1.CredentialSecretRef{
						Name: "user-password",
					},
					Temporary: false,
				},
			}
			Expect(k8sClient.Create(ctx, cred)).Should(Succeed())

			credLookupKey := types.NamespacedName{Name: credName, Namespace: namespace}
			createdCred := &keycloakv1beta1.KeycloakUserCredential{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, credLookupKey, createdCred)
				if err != nil {
					return false
				}
				return createdCred.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(k8sClient.Delete(ctx, cred)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
		})
	})
})
