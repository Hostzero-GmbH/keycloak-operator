package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	keycloakv1beta1 "github.com/Hostzero-GmbH/keycloak-operator/api/v1beta1"
	"github.com/Hostzero-GmbH/keycloak-operator/internal/keycloak"
)

const (
	reconcileNamespace = "kc"
	reconcileRealmName = "test"
)

// reconcileHarness runs a controller's Reconcile against a fake Keycloak served
// over HTTP. The Kubernetes side is a fake client holding a ready
// KeycloakInstance and KeycloakRealm that point at the fake, so the controller
// resolves its realm and talks to Keycloak exactly as it does in a cluster.
type reconcileHarness struct {
	client  client.Client
	manager *keycloak.ClientManager
}

func newReconcileHarness(t *testing.T, keycloakHandler http.Handler, objs ...client.Object) *reconcileHarness {
	t.Helper()
	srv := httptest.NewServer(keycloakHandler)
	t.Cleanup(srv.Close)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, keycloakv1beta1.AddToScheme(scheme))

	base := []client.Object{
		mkSecret("admin", reconcileNamespace, map[string]string{"username": "admin", "password": "pw"}),
		&keycloakv1beta1.KeycloakInstance{
			ObjectMeta: metav1.ObjectMeta{Name: "kci", Namespace: reconcileNamespace},
			Spec: keycloakv1beta1.KeycloakInstanceSpec{
				BaseUrl: srv.URL,
				Auth: keycloakv1beta1.AuthSpec{
					PasswordGrant: &keycloakv1beta1.PasswordGrantSpec{
						SecretRef: keycloakv1beta1.PasswordGrantSecretRefSpec{Name: "admin"},
					},
				},
			},
			Status: keycloakv1beta1.KeycloakInstanceStatus{Ready: true, Version: "26.8.0"},
		},
		&keycloakv1beta1.KeycloakRealm{
			ObjectMeta: metav1.ObjectMeta{Name: "realm", Namespace: reconcileNamespace},
			Spec:       keycloakv1beta1.KeycloakRealmSpec{InstanceRef: &keycloakv1beta1.ResourceRef{Name: "kci"}},
			Status:     keycloakv1beta1.KeycloakRealmStatus{Ready: true, RealmName: reconcileRealmName},
		},
	}

	return &reconcileHarness{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(append(base, objs...)...).
			WithStatusSubresource(
				&keycloakv1beta1.KeycloakGroup{},
				&keycloakv1beta1.KeycloakClientScope{},
				&keycloakv1beta1.KeycloakProtocolMapper{},
			).
			Build(),
		manager: keycloak.NewClientManager(logr.Discard()),
	}
}

func (h *reconcileHarness) reconcile(t *testing.T, r reconcile.Reconciler, name string) {
	t.Helper()
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Namespace: reconcileNamespace, Name: name},
	})
	require.NoError(t, err)
}

// updateObject reads obj back from the fake client, lets mutate change it and
// writes it again, like a user editing the custom resource.
func updateObject[T client.Object](t *testing.T, h *reconcileHarness, obj T, mutate func(T)) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.client.Get(ctx, client.ObjectKeyFromObject(obj), obj))
	mutate(obj)
	require.NoError(t, h.client.Update(ctx, obj))
}

// withFinalizer places meta in the harness namespace with the operator's
// finalizer already set, so the first Reconcile goes straight to Keycloak
// instead of only adding the finalizer.
func withFinalizer(meta metav1.ObjectMeta) metav1.ObjectMeta {
	meta.Namespace = reconcileNamespace
	meta.Finalizers = []string{FinalizerName}
	return meta
}

func handleToken(mux *http.ServeMux) {
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test","expires_in":300,"token_type":"Bearer"}`))
	})
}

func decodeObject(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	var obj map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &obj))
	return obj
}
