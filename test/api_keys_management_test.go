package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/handlers/management"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
)

func newAPIKeysTestHandler(t *testing.T) (*management.Handler, string) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	cfg := &config.Config{}
	if err := os.WriteFile(configPath, []byte("port: 8080\n"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	return management.NewHandler(cfg, configPath, nil), configPath
}

func setupAPIKeysRouter(h *management.Handler) *gin.Engine {
	r := gin.New()
	mgmt := r.Group("/v0/management")
	mgmt.GET("/api-keys", h.GetAPIKeys)
	mgmt.PUT("/api-keys", h.PutAPIKeys)
	mgmt.PATCH("/api-keys", h.PatchAPIKeys)
	mgmt.DELETE("/api-keys", h.DeleteAPIKeys)
	return r
}

func TestPutAPIKeys_WithStructuredEntries(t *testing.T) {
	h, configPath := newAPIKeysTestHandler(t)
	r := setupAPIKeysRouter(h)
	body := `{"value":[{"api-key":"  key-1  ","expires-at":"2026-04-01T12:00:00+08:00","token-limit":12345}]}`
	req := httptest.NewRequest(http.MethodPut, "/v0/management/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	loaded, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if len(loaded.APIKeyEntries) != 1 {
		t.Fatalf("expected 1 api-key entry, got %d", len(loaded.APIKeyEntries))
	}
	entry := loaded.APIKeyEntries[0]
	if entry.APIKey != "key-1" {
		t.Fatalf("expected key-1, got %q", entry.APIKey)
	}
	if entry.ExpiresAt != "2026-04-01T04:00:00Z" {
		t.Fatalf("expected normalized expiry, got %q", entry.ExpiresAt)
	}
	if entry.TokenLimit != 12345 {
		t.Fatalf("expected token-limit 12345, got %d", entry.TokenLimit)
	}
	if len(loaded.APIKeys) != 1 || loaded.APIKeys[0] != "key-1" {
		t.Fatalf("expected legacy api-keys sync, got %#v", loaded.APIKeys)
	}
}

func TestPatchAPIKeys_UpdatesStructuredEntry(t *testing.T) {
	h, _ := newAPIKeysTestHandler(t)
	h.SetConfig(&config.Config{SDKConfig: config.SDKConfig{APIKeyEntries: []config.APIKeyEntry{{APIKey: "key-1"}}}})
	r := setupAPIKeysRouter(h)
	body := `{"index":0,"value":{"api-key":"key-1-updated","expires-at":"2026-05-01T00:00:00Z","token-limit":99}}`
	req := httptest.NewRequest(http.MethodPatch, "/v0/management/api-keys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/v0/management/api-keys", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected GET status %d, got %d", http.StatusOK, resp.Code)
	}
	var payload struct {
		Entries []config.APIKeyEntry `json:"api-key-entries"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(payload.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(payload.Entries))
	}
	if payload.Entries[0].APIKey != "key-1-updated" || payload.Entries[0].TokenLimit != 99 {
		t.Fatalf("unexpected entry after patch: %#v", payload.Entries[0])
	}
}
