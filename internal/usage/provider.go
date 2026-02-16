package usage

import "context"

type Provider interface {
	Name() ProviderKind
	FetchUsage(ctx context.Context) (UsageSnapshot, error)
	HasCredentials() bool
}

// DefaultProviders returns the standard set of usage providers.
func DefaultProviders() []Provider {
	return []Provider{
		NewClaudeProvider(),
		NewCodexProvider(),
	}
}
