package disposable

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientReadsTokenFromFile(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	tokenFile := filepath.Join(t.TempDir(), "token")
	t.Setenv("CONTROLLER_URL", server.URL)
	t.Setenv("CONTROLLER_TOKEN", "")
	t.Setenv("CONTROLLER_TOKEN_FILE", tokenFile)

	c := FromEnv()
	if c.Configured() {
		t.Fatal("must not be configured before the controller wrote its token")
	}
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("health must fail while no token exists")
	}

	// The controller creates the token some time after this process started.
	if err := os.WriteFile(tokenFile, []byte("  generated-by-controller\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !c.Configured() {
		t.Fatal("must be configured once the token file exists")
	}
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if gotAuth != "Bearer generated-by-controller" {
		t.Fatalf("unexpected Authorization header %q", gotAuth)
	}
}

func TestClientCreateSendsConfig(t *testing.T) {
	seed := []byte(`{"defaultProvider":"openai"}`)
	var sentOwner, sentConfig string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Owner  string `json:"owner"`
			Config string `json:"config"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		sentOwner, sentConfig = body.Owner, body.Config
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"abc","name":"uagent-disp-1","ip":"172.18.0.4"}`))
	}))
	defer server.Close()

	t.Setenv("CONTROLLER_URL", server.URL)
	t.Setenv("CONTROLLER_TOKEN", "test-token")
	c := FromEnv()
	got, err := c.Create(context.Background(), "user-1", seed)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.IP != "172.18.0.4" || got.Name != "uagent-disp-1" {
		t.Fatalf("unexpected container %+v", got)
	}
	if sentOwner != "user-1" {
		t.Fatalf("owner %q", sentOwner)
	}
	decoded, err := base64.StdEncoding.DecodeString(sentConfig)
	if err != nil || string(decoded) != string(seed) {
		t.Fatalf("config not sent as base64 of the original: %q %v", sentConfig, err)
	}

	if _, err := c.Create(context.Background(), "user-1", []byte(strings.Repeat("x", MaxConfigBytes+1))); err == nil {
		t.Fatal("oversized config must be refused before calling the controller")
	}
}

func TestClientSurfacesControllerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict: limit of 8 disposable containers reached"}`))
	}))
	defer server.Close()
	t.Setenv("CONTROLLER_URL", server.URL)
	t.Setenv("CONTROLLER_TOKEN", "test-token")

	_, err := FromEnv().Create(context.Background(), "u", nil)
	var se *StatusError
	if err == nil || !errors.As(err, &se) || se.Code != http.StatusConflict {
		t.Fatalf("expected a 409 StatusError, got %v", err)
	}
}
