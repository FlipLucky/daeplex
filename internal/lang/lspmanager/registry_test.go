package lspmanager

import (
	"os"
	"reflect"
	"testing"
)

func loadFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/registry_fixture.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return data
}

// TestParseRegistryFiltersToWanted proves parseRegistry only returns the
// packages asked for, out of a fixture containing more than that.
func TestParseRegistryFiltersToWanted(t *testing.T) {
	specs, err := parseRegistry(loadFixture(t), map[string]bool{"gopls": true})
	if err != nil {
		t.Fatalf("parseRegistry() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want 1", len(specs))
	}
	if _, ok := specs["gopls"]; !ok {
		t.Errorf("specs missing %q", "gopls")
	}
}

// TestParseRegistryGoplsHasNoAssets proves an npm/go-sourced package (no
// "asset" field at all in the raw JSON) normalizes to a nil Assets slice,
// not an error or a spurious empty-but-non-nil one.
func TestParseRegistryGoplsHasNoAssets(t *testing.T) {
	specs, err := parseRegistry(loadFixture(t), map[string]bool{"gopls": true})
	if err != nil {
		t.Fatalf("parseRegistry() error = %v", err)
	}
	spec := specs["gopls"]
	if spec.Source.Assets != nil {
		t.Errorf("gopls Assets = %v, want nil (npm/go-sourced packages have no asset field)", spec.Source.Assets)
	}
	if spec.Bin["gopls"] != "golang:gopls" {
		t.Errorf(`gopls Bin["gopls"] = %q, want "golang:gopls"`, spec.Bin["gopls"])
	}
	if spec.Source.ID == "" {
		t.Error("gopls Source.ID is empty, want a real purl")
	}
}

// TestParseRegistryClangdArrayOfTargetsWithListTarget proves the
// array-of-per-target-objects shape parses correctly, including a "target"
// value that's itself a list (clangd's darwin entry covers both
// darwin_x64 and darwin_arm64 in one asset object).
func TestParseRegistryClangdArrayOfTargetsWithListTarget(t *testing.T) {
	specs, err := parseRegistry(loadFixture(t), map[string]bool{"clangd": true})
	if err != nil {
		t.Fatalf("parseRegistry() error = %v", err)
	}
	spec := specs["clangd"]
	if len(spec.Source.Assets) == 0 {
		t.Fatal("clangd has no assets, want at least one per-target entry")
	}

	var darwinAsset *Asset
	for i := range spec.Source.Assets {
		a := &spec.Source.Assets[i]
		if reflect.DeepEqual(a.Targets, []string{"darwin_x64", "darwin_arm64"}) {
			darwinAsset = a
		}
	}
	if darwinAsset == nil {
		t.Fatalf("no clangd asset covers both darwin_x64 and darwin_arm64; assets = %+v", spec.Source.Assets)
	}
	if darwinAsset.File == "" || darwinAsset.Bin == "" {
		t.Errorf("clangd darwin asset File/Bin are empty: %+v", darwinAsset)
	}
}

// TestParseRegistrySingleAssetObjectAppliesToEveryTarget proves phpactor's
// shape (a single "asset" object, not an array — the same phar for every
// platform) normalizes to one Asset with nil Targets, meaning "matches any."
func TestParseRegistrySingleAssetObjectAppliesToEveryTarget(t *testing.T) {
	specs, err := parseRegistry(loadFixture(t), map[string]bool{"phpactor": true})
	if err != nil {
		t.Fatalf("parseRegistry() error = %v", err)
	}
	spec := specs["phpactor"]
	if len(spec.Source.Assets) != 1 {
		t.Fatalf("len(phpactor.Assets) = %d, want 1", len(spec.Source.Assets))
	}
	if spec.Source.Assets[0].Targets != nil {
		t.Errorf("phpactor asset Targets = %v, want nil (applies to every target)", spec.Source.Assets[0].Targets)
	}
	if spec.Source.Assets[0].File != "phpactor.phar" {
		t.Errorf("phpactor asset File = %q, want %q", spec.Source.Assets[0].File, "phpactor.phar")
	}
}

// TestParseRegistryMarksmanFlatTargetString proves a "target" value that's
// a single string (not a list) also normalizes correctly, alongside the
// list-shaped darwin entry in the very same package.
func TestParseRegistryMarksmanFlatTargetString(t *testing.T) {
	specs, err := parseRegistry(loadFixture(t), map[string]bool{"marksman": true})
	if err != nil {
		t.Fatalf("parseRegistry() error = %v", err)
	}
	spec := specs["marksman"]

	var linuxAsset, darwinAsset *Asset
	for i := range spec.Source.Assets {
		a := &spec.Source.Assets[i]
		switch {
		case reflect.DeepEqual(a.Targets, []string{"linux_x64"}):
			linuxAsset = a
		case reflect.DeepEqual(a.Targets, []string{"darwin_x64", "darwin_arm64"}):
			darwinAsset = a
		}
	}
	if linuxAsset == nil {
		t.Errorf("no marksman asset for flat target linux_x64; assets = %+v", spec.Source.Assets)
	}
	if darwinAsset == nil {
		t.Errorf("no marksman asset for list target darwin_x64/darwin_arm64; assets = %+v", spec.Source.Assets)
	}
}

func TestParseRegistryUnwantedPackagesAreOmitted(t *testing.T) {
	specs, err := parseRegistry(loadFixture(t), map[string]bool{"gopls": true, "does-not-exist": true})
	if err != nil {
		t.Fatalf("parseRegistry() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want 1 (unknown wanted names should be silently absent, not error)", len(specs))
	}
}
