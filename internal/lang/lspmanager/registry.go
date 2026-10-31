package lspmanager

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// latestReleaseURL and the registry.json.zip asset name were both confirmed
// against a real fetch (2026-09-20) rather than assumed — mason-registry
// publishes one release per registry update, with registry.json.zip as a
// release asset (the same compiled index mason.nvim itself consumes),
// alongside the raw per-package YAML sources this package never touches.
const (
	latestReleaseURL  = "https://api.github.com/repos/mason-org/mason-registry/releases/latest"
	registryAssetName = "registry.json.zip"
)

// Asset is one normalized per-target download entry from a package's
// GitHub-release source. Targets/File/Bin are normalized here because
// mason-registry's raw JSON is polymorphic in a way that would otherwise
// leak into every caller: "target" can be a single string or a list, and a
// GitHub-sourced package's "asset" field can be a single object (one asset
// for every target, e.g. phpactor's phar) or an array of per-target objects
// (e.g. clangd, one entry per OS/arch).
type Asset struct {
	// Targets is the list of mason-registry target strings this asset
	// serves (see platform.MasonTargetCandidates) — a single-asset package
	// (no real per-target variation) normalizes to a nil Targets, meaning
	// "matches any target."
	Targets []string
	// File is the release asset filename, still template-unresolved (may
	// contain "{{version}}" or "{{ version | strip_prefix \"v\" }}").
	File string
	// Bin is the path to the executable within the downloaded asset (after
	// extraction, if it's an archive) — also template-unresolved, and may
	// self-reference "{{source.asset.file}}" when the asset itself is
	// already the raw binary (no separate archive layout to describe).
	Bin string
}

// Source describes where a package's actual binary comes from. ID is a purl
// string ("pkg:<scheme>/<namespace>/<name>@<version>") — the version is
// parsed out of it rather than tracked as a separate field, since the purl
// is the registry's own single source of truth for "which version is
// current." Assets is only populated for MethodGitHubBinary packages; NPM-
// and Go-sourced packages resolve their binary via their own package
// manager instead (see install.go), so Assets is nil for those.
type Source struct {
	ID     string
	Assets []Asset
}

// PackageSpec is one mason-registry package entry, trimmed to exactly the
// fields install.go needs — deliberately not a full mirror of the
// registry's schema (license/homepage/description/etc. are real fields on
// the raw JSON but have no install-time purpose here).
type PackageSpec struct {
	Name string
	Source
	// Bin maps an exposed executable name to its resolution scheme, e.g.
	// {"gopls": "golang:gopls"} or {"clangd": "{{source.asset.bin}}"}.
	Bin map[string]string
}

type rawTarget struct {
	Target json.RawMessage `json:"target"`
	File   string          `json:"file"`
	Bin    string          `json:"bin"`
}

type rawSource struct {
	ID    string          `json:"id"`
	Asset json.RawMessage `json:"asset"`
}

type rawPackage struct {
	Name   string            `json:"name"`
	Source rawSource         `json:"source"`
	Bin    map[string]string `json:"bin"`
}

// parseRegistry decodes a raw registry.json byte slice (the array-of-
// packages format confirmed against a real dump) into PackageSpecs, kept to
// only the package names in wanted — the curated catalog's needs, not the
// full ~600-package registry. Pure and dependency-free on purpose so it can
// be exercised in tests against a small fixture instead of live network.
func parseRegistry(data []byte, wanted map[string]bool) (map[string]PackageSpec, error) {
	var raws []rawPackage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("lspmanager: decoding registry.json: %w", err)
	}

	out := make(map[string]PackageSpec, len(wanted))
	for _, raw := range raws {
		if !wanted[raw.Name] {
			continue
		}
		assets, err := parseAssets(raw.Source.Asset)
		if err != nil {
			return nil, fmt.Errorf("lspmanager: package %q: %w", raw.Name, err)
		}
		out[raw.Name] = PackageSpec{
			Name: raw.Name,
			Source: Source{
				ID:     raw.Source.ID,
				Assets: assets,
			},
			Bin: raw.Bin,
		}
	}
	return out, nil
}

// parseAssets normalizes the "asset" field's two real shapes (a single
// object applying to every target, or an array of per-target objects) plus
// "target"'s two real shapes (a single string, or a list of strings) into a
// flat []Asset. A nil/empty raw value (npm/go/pypi/etc.-sourced packages
// have no "asset" field at all) correctly normalizes to a nil slice.
func parseAssets(raw json.RawMessage) ([]Asset, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	// Try "array of per-target objects" first.
	var list []rawTarget
	if err := json.Unmarshal(raw, &list); err == nil {
		assets := make([]Asset, len(list))
		for i, rt := range list {
			targets, err := parseTargets(rt.Target)
			if err != nil {
				return nil, err
			}
			assets[i] = Asset{Targets: targets, File: rt.File, Bin: rt.Bin}
		}
		return assets, nil
	}

	// Fall back to "single object for every target" (phpactor's shape).
	var single rawTarget
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, fmt.Errorf("unrecognized source.asset shape: %w", err)
	}
	targets, err := parseTargets(single.Target)
	if err != nil {
		return nil, err
	}
	return []Asset{{Targets: targets, File: single.File, Bin: single.Bin}}, nil
}

// parseTargets normalizes "target": "linux_x64" or "target": ["darwin_x64",
// "darwin_arm64"] into a []string; a missing "target" key normalizes to nil
// (matches any target — the single-asset-for-everyone case).
func parseTargets(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, fmt.Errorf("unrecognized target shape: %w", err)
	}
	return []string{single}, nil
}

// FetchLatestRegistryJSON hits the live mason-registry GitHub release,
// downloads its registry.json.zip asset, and returns the uncompressed
// registry.json bytes. Real network I/O — kept as a thin, separately
// callable function specifically so parseRegistry's logic (the part with
// real branching to get right) can be tested against a fixture without
// needing network access in CI.
func FetchLatestRegistryJSON(client *http.Client) ([]byte, error) {
	if client == nil {
		client = http.DefaultClient
	}

	zipURL, err := latestRegistryZipURL(client)
	if err != nil {
		return nil, err
	}

	resp, err := client.Get(zipURL)
	if err != nil {
		return nil, fmt.Errorf("lspmanager: downloading %s: %w", registryAssetName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lspmanager: downloading %s: HTTP %d", registryAssetName, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lspmanager: reading %s: %w", registryAssetName, err)
	}

	return extractRegistryJSON(body)
}

func latestRegistryZipURL(client *http.Client) (string, error) {
	resp, err := client.Get(latestReleaseURL)
	if err != nil {
		return "", fmt.Errorf("lspmanager: fetching latest release metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lspmanager: fetching latest release metadata: HTTP %d", resp.StatusCode)
	}

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("lspmanager: decoding release metadata: %w", err)
	}
	for _, a := range release.Assets {
		if a.Name == registryAssetName {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("lspmanager: latest release has no %s asset", registryAssetName)
}

func extractRegistryJSON(zipBytes []byte) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("lspmanager: opening %s: %w", registryAssetName, err)
	}
	for _, f := range r.File {
		if filepath.Base(f.Name) != "registry.json" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("lspmanager: opening registry.json inside zip: %w", err)
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("lspmanager: %s contained no registry.json", registryAssetName)
}

// LoadPackages returns PackageSpecs for wanted, from a local cache file
// under cacheDir if one exists and is younger than maxAge, otherwise
// fetching live and refreshing the cache. cacheDir is expected to come from
// platform.CacheDir() — this package doesn't import platform itself to
// avoid coupling a pure-data-fetching file to where the caller happens to
// keep its cache, matching this project's existing decoupling convention.
func LoadPackages(cacheDir string, wanted map[string]bool, maxAge time.Duration, client *http.Client) (map[string]PackageSpec, error) {
	cachePath := filepath.Join(cacheDir, "registry-cache.json")

	if info, err := os.Stat(cachePath); err == nil && time.Since(info.ModTime()) < maxAge {
		if data, err := os.ReadFile(cachePath); err == nil {
			if specs, err := parseRegistry(data, wanted); err == nil {
				return specs, nil
			}
			// Fall through to a live refetch on a corrupt/stale-schema cache
			// file rather than failing outright.
		}
	}

	data, err := FetchLatestRegistryJSON(client)
	if err != nil {
		return nil, err
	}
	specs, err := parseRegistry(data, wanted)
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(cachePath, data, 0o644) // best-effort cache write
	return specs, nil
}
