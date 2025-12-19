package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

var _ = Describe("ClusterKeycloakInstance", func() {
	const (
		instanceName = "cluster-test-instance"
	)

	Context("When creating a ClusterKeycloakInstance", func() {
		It("Should connect to Keycloak and become ready", func() {
			ctx := context.Background()

			// Create ClusterKeycloakInstance
			instance := &keycloakv1beta1.ClusterKeycloakInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name: instanceName,
				},
				Spec: keycloakv1beta1.ClusterKeycloakInstanceSpec{
					BaseUrl: "http://keycloak.keycloak.svc:8080",
					Credentials: keycloakv1beta1.ClusterCredentialsSpec{
						SecretRef: keycloakv1beta1.ClusterSecretRefSpec{
							Name:      "keycloak-credentials",
							Namespace: "default",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for instance to become ready
			instanceLookupKey := types.NamespacedName{Name: instanceName}
			createdInstance := &keycloakv1beta1.ClusterKeycloakInstance{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, instanceLookupKey, createdInstance)
				if err != nil {
					return false
				}
				return createdInstance.Status.Ready
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
