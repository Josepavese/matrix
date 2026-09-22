package agentdiscovery

import (
	"context"
	"strings"
	"testing"

	"github.com/Josepavese/matrix/internal/middleware"
)

// TestNewProviderRejectsIncompleteOptions keeps a misconfigured source from
// producing a provider that fails later, mid-discovery, with a confusing error.
// The failure has to happen where the operator can still see the cause.
func TestNewProviderRejectsIncompleteOptions(t *testing.T) {
	cases := map[string]struct {
		source Source
		opts   Options
		want   string
	}{
		"local without registry or storage": {SourceLocal, Options{}, "registry and storage"},
		"acp registry without network":      {SourceACPRegistry, Options{}, "requires network"},
		"a2a card without network":          {SourceA2ACard, Options{}, "requires network"},
		"a2a catalog without network":       {SourceA2ACatalog, Options{}, "requires network"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			provider, err := NewProvider(tc.source, tc.opts)
			if err == nil {
				t.Fatalf("incomplete options must be refused, got provider %T", provider)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q must explain what is missing (%q)", err, tc.want)
			}
		})
	}
}

// TestNewProviderRejectsACatalogWithoutAURL covers the one source whose
// requirement is not a dependency but a value: without it the provider would
// fetch from an empty address.
func TestNewProviderRejectsACatalogWithoutAURL(t *testing.T) {
	source := SourceA2ACatalog
	provider, err := NewProvider(source, Options{Net: inertNetwork{}, CatalogURL: "   "})
	if err == nil {
		t.Fatalf("a blank catalog URL must be refused, got provider %T", provider)
	}
	if !strings.Contains(err.Error(), "catalog URL") {
		t.Fatalf("the error must name the missing value, got %q", err)
	}
}

// TestNewProviderRejectsAnUnsupportedSource keeps a typo in configuration from
// silently defaulting to local discovery.
func TestNewProviderRejectsAnUnsupportedSource(t *testing.T) {
	provider, err := NewProvider(Source("nonsense"), Options{})
	if err == nil {
		t.Fatalf("an unsupported source must be refused, got provider %T", provider)
	}
	if !strings.Contains(err.Error(), "unsupported discovery source") {
		t.Fatalf("the error must name the problem, got %q", err)
	}
}

// inertNetwork satisfies middleware.Network without performing any I/O: these
// tests only exercise configuration validation, which must happen before any
// network call.
type inertNetwork struct{}

func (inertNetwork) Listen(string, string) (middleware.ClosableListener, error) {
	return nil, nil
}

func (inertNetwork) Download(context.Context, string, string) error { return nil }

func (inertNetwork) FetchJSON(context.Context, string, interface{}) error { return nil }

func (inertNetwork) GetFreePort() (int, error) { return 0, nil }

func (inertNetwork) Fetch(context.Context, string) ([]byte, error) { return nil, nil }

func (inertNetwork) PostJSON(context.Context, string, interface{}) ([]byte, int, error) {
	return nil, 0, nil
}

func (inertNetwork) CanDial(string) bool { return false }
