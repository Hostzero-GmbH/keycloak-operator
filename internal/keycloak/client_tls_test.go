package keycloak

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
)

func mkSelfSignedHTTPSServer(t *testing.T, handler http.Handler) (server *httptest.Server, caPEM string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	srv := httptest.NewUnstartedServer(handler)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{derBytes}, PrivateKey: priv}}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, string(certPEM)
}

func TestBuildTLSConfig_InsecureSkipVerify(t *testing.T) {
	cfg, ok := buildTLSConfig(Config{InsecureSkipVerify: true}, testr.New(t))
	if !ok || cfg == nil || !cfg.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify true, got %+v", cfg)
	}
}

func TestBuildTLSConfig_ValidCACert(t *testing.T) {
	_, caPEM := mkSelfSignedHTTPSServer(t, http.NewServeMux())
	cfg, ok := buildTLSConfig(Config{CACert: caPEM}, testr.New(t))
	if !ok || cfg == nil || cfg.RootCAs == nil {
		t.Fatalf("expected RootCAs populated, got %+v", cfg)
	}
}

func TestBuildTLSConfig_InvalidCACertFallsBackToSystemRoots(t *testing.T) {
	// AppendCertsFromPEM returns false for garbage; we log + fall back rather
	// than failing client construction (handler can still surface verify error).
	if _, ok := buildTLSConfig(Config{CACert: "not a pem"}, testr.New(t)); ok {
		t.Fatal("expected ok=false (fallback to system roots)")
	}
}

func TestBuildTLSConfig_None(t *testing.T) {
	if _, ok := buildTLSConfig(Config{}, testr.New(t)); ok {
		t.Fatal("expected ok=false when no TLS config requested")
	}
}

func TestClient_HTTPSWithCustomCA(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300,"token_type":"Bearer"}`))
	})
	srv, caPEM := mkSelfSignedHTTPSServer(t, mux)

	client := NewClient(Config{
		BaseURL:  srv.URL,
		Realm:    "master",
		Username: "u",
		Password: "p",
		CACert:   caPEM,
	}, testr.New(t))

	if err := client.Ping(t.Context()); err != nil {
		t.Fatalf("ping with custom CA should succeed: %v", err)
	}
}

func TestClient_HTTPSWithoutCAFailsVerification(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300,"token_type":"Bearer"}`))
	})
	srv, _ := mkSelfSignedHTTPSServer(t, mux)

	client := NewClient(Config{
		BaseURL:  srv.URL,
		Realm:    "master",
		Username: "u",
		Password: "p",
	}, testr.New(t))

	if err := client.Ping(t.Context()); err == nil {
		t.Fatal("ping without trusted CA should fail TLS verification")
	}
}

func TestClient_HTTPSInsecureSkipVerify(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300,"token_type":"Bearer"}`))
	})
	srv, _ := mkSelfSignedHTTPSServer(t, mux)

	client := NewClient(Config{
		BaseURL:            srv.URL,
		Realm:              "master",
		Username:           "u",
		Password:           "p",
		InsecureSkipVerify: true,
	}, testr.New(t))

	if err := client.Ping(t.Context()); err != nil {
		t.Fatalf("ping with InsecureSkipVerify should succeed: %v", err)
	}
}

func TestClientManager_ConfigChanged_TLSFields(t *testing.T) {
	mgr := NewClientManager(testr.New(t))
	base := Config{BaseURL: "https://kc", Username: "u", Password: "p", Realm: "master"}
	c := mgr.GetOrCreateClient("kc/i", base)

	if mgr.configChanged(c, base) {
		t.Fatal("identical config should not be detected as changed")
	}

	withCA := base
	withCA.CACert = "-----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n"
	if !mgr.configChanged(c, withCA) {
		t.Error("CACert change should trigger reconfigure")
	}

	withInsecure := base
	withInsecure.InsecureSkipVerify = true
	if !mgr.configChanged(c, withInsecure) {
		t.Error("InsecureSkipVerify change should trigger reconfigure")
	}
}
