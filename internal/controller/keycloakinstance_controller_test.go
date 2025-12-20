package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/hostzero/keycloak-operator/api/v1beta1"
)

func TestKeycloakInstanceReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = keycloakv1beta1.AddToScheme(scheme)

	tests := []struct {
		name       string
		instance   *keycloakv1beta1.KeycloakInstance
		wantRequeue bool
	}{
		{
			name: "instance not found",
			instance: nil,
			wantRequeue: false,
		},
		{
			name: "instance without finalizer",
			instance: &keycloakv1beta1.KeycloakInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: keycloakv1beta1.KeycloakInstanceSpec{
					BaseUrl: "http://keycloak:8080",
				},
			},
			wantRequeue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []runtime.Object
			if tt.instance != nil {
				objs = append(objs, tt.instance)
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithRuntimeObjects(objs...).
				Build()

			r := &KeycloakInstanceReconciler{
				Client: client,
				Scheme: scheme,
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test",
					Namespace: "default",
				},
			}

			result, err := r.Reconcile(context.Background(), req)
			
			assert.NoError(t, err)
			assert.Equal(t, tt.wantRequeue, result.Requeue)
		})
	}
}

func TestKeycloakInstanceReconciler_getKeycloakConfig(t *testing.T) {
	// Test cases for config extraction
	t.Run("missing secret should return error", func(t *testing.T) {
		scheme := runtime.NewScheme()
		_ = keycloakv1beta1.AddToScheme(scheme)

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		r := &KeycloakInstanceReconciler{
			Client: client,
			Scheme: scheme,
		}

		instance := &keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: "http://keycloak:8080",
				Credentials: keycloakv1beta1.CredentialsSpec{
					SecretRef: keycloakv1beta1.SecretRefSpec{
						Name: "nonexistent",
					},
				},
			},
		}

		_, err := r.getKeycloakConfig(context.Background(), instance)
		assert.Error(t, err)
	})
}
