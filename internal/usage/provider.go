package usage

import "context"

type Provider interface {
	Name() ProviderKind
	FetchUsage(ctx context.Context) (UsageSnapshot, error)
	HasCredentials() bool
}
