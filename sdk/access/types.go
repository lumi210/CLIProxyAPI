package access

// AccessConfig groups request authentication providers.
type AccessConfig struct {
	// Providers lists configured authentication providers.
	Providers []AccessProvider `yaml:"providers,omitempty" json:"providers,omitempty"`
}

// AccessProvider describes a request authentication provider entry.
type AccessProvider struct {
	// Name is the instance identifier for the provider.
	Name string `yaml:"name" json:"name"`

	// Type selects the provider implementation registered via the SDK.
	Type string `yaml:"type" json:"type"`

	// SDK optionally names a third-party SDK module providing this provider.
	SDK string `yaml:"sdk,omitempty" json:"sdk,omitempty"`

	// APIKeys lists legacy inline keys for providers that require them.
	APIKeys []string `yaml:"api-keys,omitempty" json:"api-keys,omitempty"`

	// APIKeyEntries lists structured inline keys with optional limits.
	APIKeyEntries []APIKeyEntry `yaml:"api-key-entries,omitempty" json:"api-key-entries,omitempty"`

	// Config passes provider-specific options to the implementation.
	Config map[string]any `yaml:"config,omitempty" json:"config,omitempty"`
}

const (
	// AccessProviderTypeConfigAPIKey is the built-in provider validating inline API keys.
	AccessProviderTypeConfigAPIKey = "config-api-key"

	// DefaultAccessProviderName is applied when no provider name is supplied.
	DefaultAccessProviderName = "config-inline"
)

// APIKeyEntry defines an inline access key with optional expiry and token limits.
type APIKeyEntry struct {
	APIKey     string `yaml:"api-key" json:"api-key"`
	ExpiresAt  string `yaml:"expires-at,omitempty" json:"expires-at,omitempty"`
	TokenLimit int64  `yaml:"token-limit,omitempty" json:"token-limit,omitempty"`
}

// MakeInlineAPIKeyProvider constructs an inline API key provider configuration.
// It returns nil when no keys are supplied.
func MakeInlineAPIKeyProvider(keys []string) *AccessProvider {
	if len(keys) == 0 {
		return nil
	}
	provider := &AccessProvider{
		Name:    DefaultAccessProviderName,
		Type:    AccessProviderTypeConfigAPIKey,
		APIKeys: append([]string(nil), keys...),
	}
	return provider
}

// MakeInlineAPIKeyProviderWithEntries constructs an inline API key provider using structured entries.
// It returns nil when no entries are supplied.
func MakeInlineAPIKeyProviderWithEntries(entries []APIKeyEntry) *AccessProvider {
	if len(entries) == 0 {
		return nil
	}
	provider := &AccessProvider{
		Name:          DefaultAccessProviderName,
		Type:          AccessProviderTypeConfigAPIKey,
		APIKeyEntries: append([]APIKeyEntry(nil), entries...),
	}
	return provider
}
