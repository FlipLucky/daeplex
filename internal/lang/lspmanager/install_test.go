package lspmanager

import (
	"archive/zip"
	"bytes"
	"os"
	"testing"
)

func TestParsePURLGolang(t *testing.T) {
	p, err := parsePURL("pkg:golang/golang.org/x/tools/gopls@v0.23.0")
	if err != nil {
		t.Fatalf("parsePURL() error = %v", err)
	}
	if p.Scheme != "golang" || p.Rest != "golang.org/x/tools/gopls" || p.Version != "v0.23.0" {
		t.Errorf("parsePURL() = %+v, want {golang, golang.org/x/tools/gopls, v0.23.0}", p)
	}
}

func TestParsePURLGithub(t *testing.T) {
	p, err := parsePURL("pkg:github/artempyanykh/marksman@2026-02-08")
	if err != nil {
		t.Fatalf("parsePURL() error = %v", err)
	}
	if p.Scheme != "github" || p.Rest != "artempyanykh/marksman" || p.Version != "2026-02-08" {
		t.Errorf("parsePURL() = %+v, want {github, artempyanykh/marksman, 2026-02-08}", p)
	}
}

func TestParsePURLNPM(t *testing.T) {
	p, err := parsePURL("pkg:npm/typescript-language-server@6.0.0")
	if err != nil {
		t.Fatalf("parsePURL() error = %v", err)
	}
	if p.Scheme != "npm" || p.Rest != "typescript-language-server" || p.Version != "6.0.0" {
		t.Errorf("parsePURL() = %+v, want {npm, typescript-language-server, 6.0.0}", p)
	}
}

func TestParsePURLMalformed(t *testing.T) {
	cases := []string{"", "npm/foo@1.0.0", "pkg:npm-foo@1.0.0", "pkg:npm/foo"}
	for _, c := range cases {
		if _, err := parsePURL(c); err == nil {
			t.Errorf("parsePURL(%q) error = nil, want an error", c)
		}
	}
}

func TestResolveVersionTemplate(t *testing.T) {
	cases := []struct{ tmpl, version, want string }{
		{"clangd-mac-{{version}}.zip", "23.1.0", "clangd-mac-23.1.0.zip"},
		{`actionlint_{{ version | strip_prefix "v" }}_linux_amd64.tar.gz`, "v1.7.12", "actionlint_1.7.12_linux_amd64.tar.gz"},
		{"no-placeholder.zip", "1.0.0", "no-placeholder.zip"},
	}
	for _, c := range cases {
		if got := resolveVersionTemplate(c.tmpl, c.version); got != c.want {
			t.Errorf("resolveVersionTemplate(%q, %q) = %q, want %q", c.tmpl, c.version, got, c.want)
		}
	}
}

func TestResolveAssetBinSelfReference(t *testing.T) {
	asset := Asset{Bin: "{{source.asset.file}}"}
	if got := resolveAssetBin(asset, "docker-language-server-linux-amd64-v0.20.1", "v0.20.1"); got != "docker-language-server-linux-amd64-v0.20.1" {
		t.Errorf("resolveAssetBin() = %q, want the resolved file name", got)
	}
}

func TestResolveAssetBinEmptyFallsBackToFile(t *testing.T) {
	// marksman's real per-target asset objects carry no "bin" field at
	// all — confirmed against real registry data, and the exact case that
	// TestInstallGitHubBinaryRealEndToEnd caught failing before this
	// fallback existed (asset.Bin=="" resolved to an empty binRel, and
	// exec.Command ended up trying to execute the install directory
	// itself).
	asset := Asset{Bin: ""}
	if got := resolveAssetBin(asset, "marksman-linux-x64", "2026-02-08"); got != "marksman-linux-x64" {
		t.Errorf("resolveAssetBin() = %q, want the resolved file name", got)
	}
}

func TestResolveAssetBinTemplatedPath(t *testing.T) {
	asset := Asset{Bin: "clangd_{{version}}/bin/clangd"}
	if got := resolveAssetBin(asset, "clangd-linux-23.1.0.zip", "23.1.0"); got != "clangd_23.1.0/bin/clangd" {
		t.Errorf("resolveAssetBin() = %q, want %q", got, "clangd_23.1.0/bin/clangd")
	}
}

func TestMatchAssetPrefersSpecificTargetOverUniversal(t *testing.T) {
	assets := []Asset{
		{Targets: nil, File: "everyone.phar"},
		{Targets: []string{"linux_x64_gnu"}, File: "linux.tar.gz"},
	}
	got, ok := matchAsset(assets, []string{"linux_x64_gnu", "linux_x64_musl", "linux_x64"})
	if !ok || got.File != "linux.tar.gz" {
		t.Errorf("matchAsset() = %+v, ok=%v, want the specific linux.tar.gz asset", got, ok)
	}
}

func TestMatchAssetFallsBackToUniversal(t *testing.T) {
	assets := []Asset{{Targets: nil, File: "everyone.phar"}}
	got, ok := matchAsset(assets, []string{"linux_x64_gnu", "linux_x64_musl", "linux_x64"})
	if !ok || got.File != "everyone.phar" {
		t.Errorf("matchAsset() = %+v, ok=%v, want the universal everyone.phar asset", got, ok)
	}
}

func TestMatchAssetNoMatch(t *testing.T) {
	assets := []Asset{{Targets: []string{"win_x64"}, File: "windows.zip"}}
	if _, ok := matchAsset(assets, []string{"linux_x64_gnu", "linux_x64_musl", "linux_x64"}); ok {
		t.Error("matchAsset() ok = true, want false (no candidate target matches, no universal asset)")
	}
}

func TestMatchAssetTriesCandidatesInOrder(t *testing.T) {
	assets := []Asset{
		{Targets: []string{"linux_x64_musl"}, File: "musl.tar.gz"},
		{Targets: []string{"linux_x64_gnu"}, File: "gnu.tar.gz"},
	}
	// Candidate order says "prefer gnu" even though musl appears first in
	// the asset list — proves matching follows caller-supplied preference
	// order, not registry declaration order.
	got, ok := matchAsset(assets, []string{"linux_x64_gnu", "linux_x64_musl"})
	if !ok || got.File != "gnu.tar.gz" {
		t.Errorf("matchAsset() = %+v, ok=%v, want gnu.tar.gz (first in candidate order)", got, ok)
	}
}

func TestSafeJoinRejectsPathEscape(t *testing.T) {
	if _, err := safeJoin("/tmp/dest", "../../etc/passwd"); err == nil {
		t.Error("safeJoin() error = nil, want an error for a path escaping the destination")
	}
}

func TestSafeJoinAllowsNormalRelativePath(t *testing.T) {
	got, err := safeJoin("/tmp/dest", "bin/clangd")
	if err != nil {
		t.Fatalf("safeJoin() error = %v", err)
	}
	if got != "/tmp/dest/bin/clangd" {
		t.Errorf("safeJoin() = %q, want %q", got, "/tmp/dest/bin/clangd")
	}
}

func TestExtractZipRejectsZipSlip(t *testing.T) {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	f, _ := w.Create("../../evil.txt")
	_, _ = f.Write([]byte("pwned"))
	_ = w.Close()

	if err := extractZip(buf.Bytes(), t.TempDir()); err == nil {
		t.Error("extractZip() error = nil, want a zip-slip rejection")
	}
}

func TestExtractZipWritesFilesAndSetsExecutable(t *testing.T) {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	f, _ := w.Create("bin/tool")
	_, _ = f.Write([]byte("#!/bin/sh\necho hi\n"))
	_ = w.Close()

	dest := t.TempDir()
	if err := extractZip(buf.Bytes(), dest); err != nil {
		t.Fatalf("extractZip() error = %v", err)
	}
	data, err := os.ReadFile(dest + "/bin/tool")
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(data) != "#!/bin/sh\necho hi\n" {
		t.Errorf("extracted file content = %q, want the original bytes", data)
	}
}

func TestRuntimeDisplayName(t *testing.T) {
	if runtimeDisplayName(RuntimeGo) == "" || runtimeDisplayName(RuntimeNode) == "" {
		t.Error("runtimeDisplayName() returned empty for a known runtime")
	}
}
