package unleash

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LOKE/pkg/featureflags"
	unleashsdk "github.com/Unleash/unleash-go-sdk/v5"
	unleashcontext "github.com/Unleash/unleash-go-sdk/v5/context"
	"github.com/Unleash/unleash-go-sdk/v5/strategy"
)

func TestStrategies(t *testing.T) {
	ctxWith := func(key, value string) *unleashcontext.Context {
		return &unleashcontext.Context{Properties: map[string]string{key: value}}
	}
	converted := func(ctx featureflags.Context) *unleashcontext.Context {
		c := convertContext(ctx)
		return &c
	}

	tests := []struct {
		name     string
		strategy strategy.Strategy
		params   map[string]any
		ctx      *unleashcontext.Context
		want     bool
	}{
		{"blacklist listed org", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a, org-b"}, converted(featureflags.Context{OrgID: "org-a"}), false},
		{"blacklist other org", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a, org-b"}, ctxWith("orgId", "org-c"), true},
		{"blacklist no org", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a"}, ctxWith("orgId", ""), true},
		{"blacklist nil context", BlacklistOrgWithID{}, map[string]any{"orgIds": "org-a"}, nil, true},
		{"org listed", OrgWithID{}, map[string]any{"orgIds": "org-a,org-b"}, converted(featureflags.Context{OrgID: "org-b"}), true},
		{"org missing param", OrgWithID{}, map[string]any{}, ctxWith("orgId", "org-a"), false},
		{"currency listed", Currency{}, map[string]any{"currencies": "AUD, NZD"}, converted(featureflags.Context{Currency: "NZD"}), true},
		{"currency unlisted", Currency{}, map[string]any{"currencies": "AUD, NZD"}, ctxWith("currency", "USD"), false},
		{"currency empty", Currency{}, map[string]any{"currencies": "AUD"}, ctxWith("currency", ""), false},
		{"currency nil context", Currency{}, map[string]any{"currencies": "AUD"}, nil, false},
		{"device listed", DeviceID{}, map[string]any{"deviceIds": "d-1"}, converted(featureflags.Context{DeviceID: "d-1"}), true},
		{"client listed", ClientWithID{}, map[string]any{"clientIds": "c-1"}, converted(featureflags.Context{ClientID: "c-1"}), true},
		{"client prefix", ClientWithIDPrefix{}, map[string]any{"prefixes": "pos-, kiosk-"}, converted(featureflags.Context{ClientID: "kiosk-7"}), true},
		{"client no prefix match", ClientWithIDPrefix{}, map[string]any{"prefixes": "pos-"}, converted(featureflags.Context{ClientID: "kiosk-7"}), false},
		{"location listed", LocationWithID{}, map[string]any{"locationIds": "l-1,l-2"}, converted(featureflags.Context{LocationID: "l-2"}), true},
		{"email listed", UserWithEmail{}, map[string]any{"emails": "a@example.com"}, converted(featureflags.Context{Email: "a@example.com"}), true},
		{"email unlisted", UserWithEmail{}, map[string]any{"emails": "a@example.com"}, converted(featureflags.Context{Email: "b@example.com"}), false},
		{"timestamp past", Timestamp{}, map[string]any{"enableAfter": "2000-01-01T00:00:00Z"}, nil, true},
		{"timestamp future", Timestamp{}, map[string]any{"enableAfter": "2999-01-01T00:00:00Z"}, nil, false},
		{"timestamp invalid", Timestamp{}, map[string]any{"enableAfter": "soon"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.strategy.IsEnabled(tt.params, tt.ctx); got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAsProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/client/features" {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":2,"features":[{"name":"org-flag","enabled":true,"strategies":[{"name":"orgWithId","parameters":{"orgIds":"org-a"}}]}]}`))
	}))
	defer server.Close()

	client, err := unleashsdk.NewClient(
		unleashsdk.WithAppName("test"),
		unleashsdk.WithUrl(server.URL+"/"),
		unleashsdk.WithDisableMetrics(true),
		unleashsdk.WithStrategies(Strategies()...),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() { _ = client.Close() }()
	select {
	case <-client.Ready():
	case <-time.After(5 * time.Second):
		t.Fatal("client never became ready")
	}

	provider := AsProvider(client)
	if !provider.IsEnabled("org-flag", featureflags.Context{OrgID: "org-a"}) {
		t.Error("org-flag disabled for listed org")
	}
	if provider.IsEnabled("org-flag", featureflags.Context{OrgID: "org-b"}) {
		t.Error("org-flag enabled for unlisted org")
	}
	if provider.IsEnabled("missing-flag", featureflags.Context{OrgID: "org-a"}) {
		t.Error("unknown flag enabled")
	}
}
