package lspmanager

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// poisonedClient returns an *http.Client that fails any real request it's
// asked to make — used to prove a cache-hit path genuinely never reaches
// the network, rather than just happening to be fast.
func poisonedClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("poisonedClient: no network access should have been needed")
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestFetchLatestRegistryJSONLive proves the real network path — GitHub
// release lookup, registry.json.zip download, zip extraction — actually
// works against the live mason-registry, not just the pure parser tested
// elsewhere against a fixture. Skipped by default (this project's tests run
// with no network dependency; see the pty-verification discipline in
// CLAUDE.md) — set LSPMANAGER_INTEGRATION=1 to run it for real.
func TestFetchLatestRegistryJSONLive(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run this against the live mason-registry")
	}

	data, err := FetchLatestRegistryJSON(nil)
	if err != nil {
		t.Fatalf("FetchLatestRegistryJSON() error = %v", err)
	}
	specs, err := parseRegistry(data, map[string]bool{"gopls": true, "clangd": true, "marksman": true})
	if err != nil {
		t.Fatalf("parseRegistry() on live data error = %v", err)
	}
	for _, name := range []string{"gopls", "clangd", "marksman"} {
		if _, ok := specs[name]; !ok {
			t.Errorf("live registry is missing expected package %q", name)
		}
	}
	if specs["gopls"].Bin["gopls"] == "" {
		t.Error("live gopls entry has no bin mapping")
	}
	if len(specs["clangd"].Source.Assets) == 0 {
		t.Error("live clangd entry has no assets")
	}
}

func TestLoadPackagesUsesCacheOnSecondCallLive(t *testing.T) {
	if os.Getenv("LSPMANAGER_INTEGRATION") == "" {
		t.Skip("set LSPMANAGER_INTEGRATION=1 to run this against the live mason-registry")
	}

	dir := t.TempDir()
	wanted := map[string]bool{"gopls": true}

	first, err := LoadPackages(dir, wanted, time.Hour, nil)
	if err != nil {
		t.Fatalf("LoadPackages() first call error = %v", err)
	}
	if _, ok := first["gopls"]; !ok {
		t.Fatal("first LoadPackages() call missing gopls")
	}

	// Second call within maxAge must not need network — proved indirectly
	// by using an http.Client that errors on any real request.
	second, err := LoadPackages(dir, wanted, time.Hour, poisonedClient())
	if err != nil {
		t.Fatalf("LoadPackages() second call (should be cache-only) error = %v", err)
	}
	if _, ok := second["gopls"]; !ok {
		t.Fatal("second LoadPackages() call missing gopls")
	}
}
