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

var _ = Describe("KeycloakInstance", func() {
	const (
		instanceName = "test-instance"
		namespace    = "default"
	)

	Context("When creating a KeycloakInstance", func() {
		It("Should connect to Keycloak and become ready", func() {
			ctx := context.Background()

			// Create credentials secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "keycloak-credentials",
					Namespace: namespace,
				},
				StringData: map[string]string{
					"username": "admin",
					"password": "admin",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).Should(Succeed())

			// Create KeycloakInstance
			instance := &keycloakv1beta1.KeycloakInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
				},
				Spec: keycloakv1beta1.KeycloakInstanceSpec{
					BaseUrl: "http://keycloak.keycloak.svc:8080",
					Credentials: keycloakv1beta1.CredentialsSpec{
						SecretRef: keycloakv1beta1.SecretRefSpec{
							Name: "keycloak-credentials",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for instance to become ready
			instanceLookupKey := types.NamespacedName{Name: instanceName, Namespace: namespace}
			createdInstance := &keycloakv1beta1.KeycloakInstance{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, createdInstance)
				if err != nil {
					return false
				}
				return createdInstance.Status.Ready
			}, timeout, interval).Should(BeTrue())

			Expect(createdInstance.Status.Version).ShouldNot(BeEmpty())

			// Cleanup
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
			Expect(k8sClient.Delete(ctx, secret)).Should(Succeed())
		})
	})
})
