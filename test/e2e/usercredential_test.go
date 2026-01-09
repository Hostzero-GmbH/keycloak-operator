package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

func TestKeycloakUserCredentialE2E(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "usercred")

	t.Run("CreateSecretForUser", func(t *testing.T) {
		// First create a user
		userName := fmt.Sprintf("cred-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"email": "%s@example.com",
			"enabled": true
		}`, userName, userName))

		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait for user to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUser did not become ready")

		// Create UserCredential with create=true
		credName := fmt.Sprintf("%s-cred", userName)
		secretName := fmt.Sprintf("%s-secret", userName)
		kcCred := &keycloakv1beta1.KeycloakUserCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      credName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserCredentialSpec{
				UserRef: keycloakv1beta1.ResourceRef{Name: userName},
				UserSecret: keycloakv1beta1.CredentialSecretSpec{
					SecretName:  secretName,
					Create:      true,
					UsernameKey: "username",
					PasswordKey: "password",
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcCred))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcCred)
		})

		// Wait for credential to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUserCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcCred.Name,
				Namespace: kcCred.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUserCredential did not become ready")

		// Verify secret was created
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: testNamespace,
		}, secret)
		require.NoError(t, err, "Secret was not created")

		// Verify secret has required keys
		require.Contains(t, secret.Data, "username", "Secret missing username key")
		require.Contains(t, secret.Data, "password", "Secret missing password key")
		require.NotEmpty(t, secret.Data["password"], "Password should not be empty")

		// Verify status
		updatedCred := &keycloakv1beta1.KeycloakUserCredential{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcCred.Name,
			Namespace: kcCred.Namespace,
		}, updatedCred))
		require.True(t, updatedCred.Status.SecretCreated, "SecretCreated should be true")
	})

	t.Run("UseExistingSecret", func(t *testing.T) {
		// Create a user
		userName := fmt.Sprintf("existing-secret-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"enabled": true
		}`, userName))

		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait for user to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Create secret first
		secretName := fmt.Sprintf("%s-existing-secret", userName)
		existingSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: testNamespace,
			},
			Data: map[string][]byte{
				"username": []byte(userName),
				"password": []byte("my-custom-password-123"),
			},
		}
		require.NoError(t, k8sClient.Create(ctx, existingSecret))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, existingSecret)
		})

		// Create UserCredential with create=false (use existing)
		credName := fmt.Sprintf("%s-cred-existing", userName)
		kcCred := &keycloakv1beta1.KeycloakUserCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      credName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserCredentialSpec{
				UserRef: keycloakv1beta1.ResourceRef{Name: userName},
				UserSecret: keycloakv1beta1.CredentialSecretSpec{
					SecretName:  secretName,
					Create:      false, // Use existing
					UsernameKey: "username",
					PasswordKey: "password",
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcCred))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcCred)
		})

		// Wait for credential to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUserCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcCred.Name,
				Namespace: kcCred.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err, "KeycloakUserCredential did not become ready with existing secret")

		// Verify status - SecretCreated should be false
		updatedCred := &keycloakv1beta1.KeycloakUserCredential{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcCred.Name,
			Namespace: kcCred.Namespace,
		}, updatedCred))
		require.False(t, updatedCred.Status.SecretCreated, "SecretCreated should be false for existing secret")
	})

	t.Run("MissingSecretError", func(t *testing.T) {
		// Create a user
		userName := fmt.Sprintf("missing-secret-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"enabled": true
		}`, userName))

		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait for user to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Create UserCredential referencing non-existent secret with create=false
		credName := fmt.Sprintf("%s-cred-missing", userName)
		kcCred := &keycloakv1beta1.KeycloakUserCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      credName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserCredentialSpec{
				UserRef: keycloakv1beta1.ResourceRef{Name: userName},
				UserSecret: keycloakv1beta1.CredentialSecretSpec{
					SecretName: "non-existent-secret",
					Create:     false,
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcCred))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcCred)
		})

		// Wait a bit and check status - should be in error state
		time.Sleep(5 * time.Second)
		updatedCred := &keycloakv1beta1.KeycloakUserCredential{}
		require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{
			Name:      kcCred.Name,
			Namespace: kcCred.Namespace,
		}, updatedCred))

		require.False(t, updatedCred.Status.Ready, "Should not be ready when secret is missing")
		require.Equal(t, "SecretError", updatedCred.Status.Status)
	})
}

func TestKeycloakUserCredentialCleanup(t *testing.T) {
	skipIfNoCluster(t)

	instanceName, instanceNS := getOrCreateInstance(t)
	realmName := createTestRealm(t, instanceName, instanceNS, "usercred-cleanup")

	t.Run("SecretDeletedWithCredential", func(t *testing.T) {
		// Create a user
		userName := fmt.Sprintf("cleanup-user-%d", time.Now().UnixNano())
		userDef := rawJSON(fmt.Sprintf(`{
			"username": "%s",
			"enabled": true
		}`, userName))

		kcUser := &keycloakv1beta1.KeycloakUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserSpec{
				RealmRef:   &keycloakv1beta1.ResourceRef{Name: realmName},
				Definition: &userDef,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcUser))
		t.Cleanup(func() {
			k8sClient.Delete(ctx, kcUser)
		})

		// Wait for user to be ready
		err := wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUser{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcUser.Name,
				Namespace: kcUser.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Create UserCredential with create=true
		credName := fmt.Sprintf("%s-cleanup-cred", userName)
		secretName := fmt.Sprintf("%s-cleanup-secret", userName)
		kcCred := &keycloakv1beta1.KeycloakUserCredential{
			ObjectMeta: metav1.ObjectMeta{
				Name:      credName,
				Namespace: testNamespace,
			},
			Spec: keycloakv1beta1.KeycloakUserCredentialSpec{
				UserRef: keycloakv1beta1.ResourceRef{Name: userName},
				UserSecret: keycloakv1beta1.CredentialSecretSpec{
					SecretName: secretName,
					Create:     true,
				},
			},
		}
		require.NoError(t, k8sClient.Create(ctx, kcCred))

		// Wait for credential to be ready
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			updated := &keycloakv1beta1.KeycloakUserCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcCred.Name,
				Namespace: kcCred.Namespace,
			}, updated); err != nil {
				return false, nil
			}
			return updated.Status.Ready, nil
		})
		require.NoError(t, err)

		// Verify secret exists
		secret := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: testNamespace,
		}, secret)
		require.NoError(t, err, "Secret should exist")

		// Delete the credential
		require.NoError(t, k8sClient.Delete(ctx, kcCred))

		// Wait for credential to be deleted
		err = wait.PollUntilContextTimeout(ctx, interval, timeout, true, func(ctx context.Context) (bool, error) {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      kcCred.Name,
				Namespace: kcCred.Namespace,
			}, &keycloakv1beta1.KeycloakUserCredential{})
			return errors.IsNotFound(err), nil
		})
		require.NoError(t, err)

		// Secret should also be deleted (owned by credential)
		err = k8sClient.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: testNamespace,
		}, secret)
		require.True(t, errors.IsNotFound(err), "Secret should be deleted with credential")
	})
}
