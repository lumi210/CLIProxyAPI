package configaccess

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)

func TestNormalizeAPIKeyEntries_MergesFallback(t *testing.T) {
	entries := normalizeAPIKeyEntries([]config.APIKeyEntry{{APIKey: " key-1 ", TokenLimit: 10}}, []string{"key-2", "key-1"})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].APIKey != "key-1" || entries[0].TokenLimit != 10 {
		t.Fatalf("unexpected first entry: %#v", entries[0])
	}
	if entries[1].APIKey != "key-2" {
		t.Fatalf("unexpected fallback entry: %#v", entries[1])
	}
}

func TestProviderAuthenticate_RejectsExpiredKey(t *testing.T) {
	p := newProvider("inline", []config.APIKeyEntry{{APIKey: "expired-key", ExpiresAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)}})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer expired-key")
	_, err := p.Authenticate(req.Context(), req)
	if err == nil {
		t.Fatal("expected expired key to be rejected")
	}
}

func TestProviderAuthenticate_RejectsTokenLimitExceededKey(t *testing.T) {
	stats := usage.GetRequestStatistics()
	stats.Record(nil, usageRecord("limited-key", 100))
	p := newProvider("inline", []config.APIKeyEntry{{APIKey: "limited-key", TokenLimit: 50}})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer limited-key")
	_, err := p.Authenticate(req.Context(), req)
	if err == nil {
		t.Fatal("expected over-limit key to be rejected")
	}
}

func usageRecord(key string, total int64) coreusage.Record {
	return coreusage.Record{
		APIKey: key,
		Detail: coreusage.Detail{TotalTokens: total},
	}
}
