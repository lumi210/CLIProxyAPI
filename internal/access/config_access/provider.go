package configaccess

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
	sdkaccess "github.com/router-for-me/CLIProxyAPI/v6/sdk/access"
	sdkconfig "github.com/router-for-me/CLIProxyAPI/v6/sdk/config"
)

// Register ensures the config-access provider is available to the access manager.
func Register(cfg *sdkconfig.SDKConfig) {
	if cfg == nil {
		sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey)
		return
	}

	entries := normalizeAPIKeyEntries(cfg.APIKeyEntries, cfg.APIKeys)
	if len(entries) == 0 {
		sdkaccess.UnregisterProvider(sdkaccess.AccessProviderTypeConfigAPIKey)
		return
	}

	sdkaccess.RegisterProvider(
		sdkaccess.AccessProviderTypeConfigAPIKey,
		newProvider(sdkaccess.DefaultAccessProviderName, entries),
	)
}

type provider struct {
	name string
	keys map[string]apiKeyPolicy
}

type apiKeyPolicy struct {
	expiresAt  time.Time
	tokenLimit int64
}

func newProvider(name string, entries []sdkconfig.APIKeyEntry) *provider {
	providerName := strings.TrimSpace(name)
	if providerName == "" {
		providerName = sdkaccess.DefaultAccessProviderName
	}
	keySet := make(map[string]apiKeyPolicy, len(entries))
	for _, entry := range entries {
		policy := apiKeyPolicy{tokenLimit: entry.TokenLimit}
		if entry.ExpiresAt != "" {
			if parsed, err := time.Parse(time.RFC3339, entry.ExpiresAt); err == nil {
				policy.expiresAt = parsed
			}
		}
		keySet[entry.APIKey] = policy
	}
	return &provider{name: providerName, keys: keySet}
}

func (p *provider) Identifier() string {
	if p == nil || p.name == "" {
		return sdkaccess.DefaultAccessProviderName
	}
	return p.name
}

func (p *provider) Authenticate(_ context.Context, r *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	if p == nil {
		return nil, sdkaccess.NewNotHandledError()
	}
	if len(p.keys) == 0 {
		return nil, sdkaccess.NewNotHandledError()
	}
	authHeader := r.Header.Get("Authorization")
	authHeaderGoogle := r.Header.Get("X-Goog-Api-Key")
	authHeaderAnthropic := r.Header.Get("X-Api-Key")
	queryKey := ""
	queryAuthToken := ""
	if r.URL != nil {
		queryKey = r.URL.Query().Get("key")
		queryAuthToken = r.URL.Query().Get("auth_token")
	}
	if authHeader == "" && authHeaderGoogle == "" && authHeaderAnthropic == "" && queryKey == "" && queryAuthToken == "" {
		return nil, sdkaccess.NewNoCredentialsError()
	}

	apiKey := extractBearerToken(authHeader)

	candidates := []struct {
		value  string
		source string
	}{
		{apiKey, "authorization"},
		{authHeaderGoogle, "x-goog-api-key"},
		{authHeaderAnthropic, "x-api-key"},
		{queryKey, "query-key"},
		{queryAuthToken, "query-auth-token"},
	}

	for _, candidate := range candidates {
		if candidate.value == "" {
			continue
		}
		policy, ok := p.keys[candidate.value]
		if !ok {
			continue
		}
		if !policy.expiresAt.IsZero() && !policy.expiresAt.After(time.Now()) {
			return nil, sdkaccess.NewInvalidCredentialError()
		}
		if policy.tokenLimit > 0 {
			stats := usage.GetRequestStatistics().Snapshot()
			apiStats, exists := stats.APIs[candidate.value]
			if exists && apiStats.TotalTokens >= policy.tokenLimit {
				return nil, sdkaccess.NewInvalidCredentialError()
			}
		}
		{
			return &sdkaccess.Result{
				Provider:  p.Identifier(),
				Principal: candidate.value,
				Metadata: map[string]string{
					"source": candidate.source,
				},
			}, nil
		}
	}

	return nil, sdkaccess.NewInvalidCredentialError()
}

func extractBearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return header
	}
	if strings.ToLower(parts[0]) != "bearer" {
		return header
	}
	return strings.TrimSpace(parts[1])
}

func normalizeKeys(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		if _, exists := seen[trimmedKey]; exists {
			continue
		}
		seen[trimmedKey] = struct{}{}
		normalized = append(normalized, trimmedKey)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeAPIKeyEntries(entries []sdkconfig.APIKeyEntry, fallback []string) []sdkconfig.APIKeyEntry {
	seen := make(map[string]struct{}, len(entries)+len(fallback))
	normalized := make([]sdkconfig.APIKeyEntry, 0, len(entries)+len(fallback))
	for _, entry := range entries {
		key := strings.TrimSpace(entry.APIKey)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		entry.APIKey = key
		entry.ExpiresAt = strings.TrimSpace(entry.ExpiresAt)
		if entry.TokenLimit < 0 {
			entry.TokenLimit = 0
		}
		normalized = append(normalized, entry)
	}
	for _, key := range normalizeKeys(fallback) {
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, sdkconfig.APIKeyEntry{APIKey: key})
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}
