package unleash

import (
	"testing"

	"github.com/LOKE/pkg/featureflags"
	"github.com/Unleash/unleash-go-sdk/v5/context"
	"github.com/Unleash/unleash-go-sdk/v5/strategy"
)

func TestStrategies(t *testing.T) {
	ctxWith := func(key, value string) *context.Context {
		return &context.Context{Properties: map[string]string{key: value}}
	}
	converted := func(ctx featureflags.Context) *context.Context {
		c := convertContext(ctx)
		return &c
	}

	tests := []struct {
		name     string
		strategy strategy.Strategy
		params   map[string]any
		ctx      *context.Context
		want     bool
	}{
		{"blacklist listed org", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a, org-b"}, ctxWith("orgId", "org-a"), false},
		{"blacklist other org", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a, org-b"}, ctxWith("orgId", "org-c"), true},
		{"blacklist no org", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a"}, ctxWith("orgId", ""), true},
		{"blacklist nil context", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a"}, nil, true},
		{"currency listed", Currency{}, map[string]any{"currencies": "AUD, NZD"}, ctxWith("currency", "NZD"), true},
		{"currency unlisted", Currency{}, map[string]any{"currencies": "AUD, NZD"}, ctxWith("currency", "USD"), false},
		{"currency empty", Currency{}, map[string]any{"currencies": "AUD"}, ctxWith("currency", ""), false},
		{"currency nil context", Currency{}, map[string]any{"currencies": "AUD"}, nil, false},
		{"org missing param", OrgWithID{}, map[string]any{}, ctxWith("orgId", "org-a"), false},
		{"client from context", ClientWithID{}, map[string]any{"clientIds": "c-1"}, converted(featureflags.Context{ClientID: "c-1"}), true},
		{"client prefix from context", ClientWithIDPrefix{}, map[string]any{"prefixes": "pos-, kiosk-"}, converted(featureflags.Context{ClientID: "kiosk-7"}), true},
		{"location from context", LocationWithID{}, map[string]any{"locationIds": "l-1,l-2"}, converted(featureflags.Context{LocationID: "l-2"}), true},
		{"email from context", UserWithEmail{}, map[string]any{"emails": "a@example.com"}, converted(featureflags.Context{Email: "b@example.com"}), false},
		{"timestamp past", Timestamp{}, map[string]any{"enableAfter": "2000-01-01T00:00:00Z"}, nil, true},
		{"timestamp future", Timestamp{}, map[string]any{"enableAfter": "2999-01-01T00:00:00Z"}, nil, false},
		{"timestamp invalid", Timestamp{}, map[string]any{"enableAfter": "soon"}, nil, false},
	}
	for _, tt := range tests {
		if got := tt.strategy.IsEnabled(tt.params, tt.ctx); got != tt.want {
			t.Errorf("%s: IsEnabled() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
