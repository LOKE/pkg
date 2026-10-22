// Package featureflags defines a provider-agnostic way for services to check
// feature flags. Use the unleash subpackage for a Provider backed by Unleash,
// and AllEnabled or AllDisabled in tests.
package featureflags

// Context holds the attributes a flag can be targeted on. Leave a field empty
// when it doesn't apply to the request.
type Context struct {
	OrgID         string
	UserID        string
	Email         string
	DeviceID      string
	LocationID    string
	ClientID      string
	Currency      string
	RemoteAddress string
}

// Provider reports whether a flag is enabled for a Context.
type Provider interface {
	IsEnabled(flag string, ctx Context) bool
}

type boolProvider bool

// AllEnabled and AllDisabled are fixed Providers for tests and local runs.
const (
	AllEnabled  boolProvider = true
	AllDisabled boolProvider = false
)

func (r boolProvider) IsEnabled(_ string, _ Context) bool {
	return bool(r)
}
