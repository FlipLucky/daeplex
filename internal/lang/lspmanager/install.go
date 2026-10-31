package lspmanager

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// InstalledServer is what Install returns on success — enough for
// internal/lspclient to spawn the server directly, no further resolution
// needed.
type InstalledServer struct {
	PackageName string
	BinName     string
	BinPath     string
	Version     string
}

// purl is deliberately not a full package-url-spec implementation — only
// what the curated catalog's actual source IDs need. Confirmed against
// real registry data (2026-09-20) that none of the curated packages use a
// scoped/percent-encoded name (e.g. "@foo/bar"), which real purl parsing
// would need to handle; if a future catalog entry does, this needs
// revisiting rather than silently mis-parsing.
type purl struct {
	Scheme  string // "golang", "npm", "github", etc.
	Rest    string // everything between scheme and version, e.g. "golang.org/x/tools/gopls" or "artempyanykh/marksman"
	Version string
}

func parsePURL(id string) (purl, error) {
	const prefix = "pkg:"
	if !strings.HasPrefix(id, prefix) {
		return purl{}, fmt.Errorf("lspmanager: not a purl: %q", id)
	}
	scheme, rest, ok := strings.Cut(id[len(prefix):], "/")
	if !ok {
		return purl{}, fmt.Errorf("lspmanager: malformed purl (no scheme): %q", id)
	}
	nameAndVersion, version, ok := cutLast(rest, "@")
	if !ok {
		return purl{}, fmt.Errorf("lspmanager: malformed purl (no @version): %q", id)
	}
	return purl{Scheme: scheme, Rest: nameAndVersion, Version: version}, nil
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func runtimeDisplayName(r Runtime) string {
	switch r {
	case RuntimeGo:
		return "a Go toolchain"
	case RuntimeNode:
		return "Node.js"
	default:
		return "an unknown runtime"
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// Install resolves spec+entry into a real, runnable server under
// <serversDir>/<package>/<version>/, checking entry's declared runtime
// dependency first — per the product decision behind this whole package,
// that check happens here, lazily, only for the specific server being
// installed, never as a blanket upfront requirement. targets is normally
// platform.CurrentTargets(); passed in rather than computed here so this
// function stays testable against a fake asset list for an arbitrary
// platform.
func Install(serversDir string, spec PackageSpec, entry Entry, targets []string) (InstalledServer, error) {
	if name := entry.Runtime.lookPathName(); name != "" {
		if _, err := exec.LookPath(name); err != nil {
			return InstalledServer{}, fmt.Errorf(
				"%s needs %s (%q not found on PATH) — install it, then retry installing %s",
				entry.PackageName, runtimeDisplayName(entry.Runtime), name, entry.PackageName)
		}
	}

	if entry.Method == MethodNone {
		found, err := exec.LookPath(entry.BinName)
		if err != nil {
			return InstalledServer{}, fmt.Errorf(
				"%s not found on PATH — it ships with its own SDK, nothing for DaePlex to install; make sure the SDK is installed and on PATH", entry.BinName)
		}
		return InstalledServer{PackageName: entry.PackageName, BinName: entry.BinName, BinPath: found}, nil
	}

	p, err := parsePURL(spec.Source.ID)
	if err != nil {
		return InstalledServer{}, err
	}

	installDir := filepath.Join(serversDir, spec.Name, p.Version)
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return InstalledServer{}, fmt.Errorf("lspmanager: creating install dir: %w", err)
	}

	var binPath string
	switch entry.Method {
	case MethodGo:
		binPath, err = installGo(installDir, p, entry.BinName)
	case MethodNPM:
		binPath, err = installNPM(installDir, p, entry.BinName)
	case MethodGitHubBinary:
		binPath, err = installGitHubBinary(installDir, spec, p, targets)
	default:
		return InstalledServer{}, fmt.Errorf("lspmanager: unknown install method %v for %s", entry.Method, spec.Name)
	}
	if err != nil {
		return InstalledServer{}, err
	}

	info, statErr := os.Stat(binPath)
	if statErr != nil {
		return InstalledServer{}, fmt.Errorf("lspmanager: %s installed but %s is missing: %w", spec.Name, binPath, statErr)
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		_ = os.Chmod(binPath, info.Mode()|0o111)
	}

	return InstalledServer{PackageName: spec.Name, BinName: entry.BinName, BinPath: binPath, Version: p.Version}, nil
}

func installGo(installDir string, p purl, binName string) (string, error) {
	cmd := exec.Command("go", "install", p.Rest+"@"+p.Version)
	cmd.Env = append(os.Environ(), "GOBIN="+installDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("lspmanager: go install %s@%s: %w\n%s", p.Rest, p.Version, err, out)
	}
	return filepath.Join(installDir, binName+exeSuffix()), nil
}

// installNPM's binary-location convention (<prefix>/node_modules/.bin/<bin>)
// was confirmed against a real `npm install --prefix` run, not assumed.
// Known limitation, not solved here: on Windows, npm generates a .cmd/.ps1
// shim rather than a directly-exec-able binary at that path — real support
// for installing npm-sourced servers on Windows needs revisiting this, not
// silently mishandling it.
func installNPM(installDir string, p purl, binName string) (string, error) {
	pkgSpec := p.Rest + "@" + p.Version
	cmd := exec.Command("npm", "install", "--prefix", installDir, "--no-audit", "--no-fund", "--loglevel=error", pkgSpec)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("lspmanager: npm install %s: %w\n%s", pkgSpec, err, out)
	}
	return filepath.Join(installDir, "node_modules", ".bin", binName), nil
}

var (
	versionPipeRe = regexp.MustCompile(`\{\{\s*version\s*\|\s*strip_prefix\s*"v"\s*\}\}`)
	versionRe     = regexp.MustCompile(`\{\{\s*version\s*\}\}`)
)

// resolveVersionTemplate substitutes mason-registry's two real version
// placeholders — confirmed against real asset templates like
// "clangd-mac-{{version}}.zip" and "actionlint_{{ version | strip_prefix
// \"v\" }}_linux_amd64.tar.gz" — into a concrete string. Does not handle
// the separate "{{source.asset.*}}" self-reference forms; those are
// resolved explicitly by resolveAssetBin instead, since they only ever
// appear as an entire field value, never embedded in a larger template.
func resolveVersionTemplate(tmpl, version string) string {
	stripped := strings.TrimPrefix(version, "v")
	tmpl = versionPipeRe.ReplaceAllString(tmpl, stripped)
	tmpl = versionRe.ReplaceAllString(tmpl, version)
	return tmpl
}

// resolveAssetBin resolves an asset's binary path relative to installDir.
// Two real shapes need the same "just use the downloaded file" fallback,
// confirmed against real data: an asset can omit "bin" entirely (marksman —
// there's no separate archive layout, the download *is* the binary), or
// carry it explicitly as the literal self-reference template
// "{{source.asset.file}}" (docker-language-server). Only a real templated
// path (clangd's "clangd_{{version}}/bin/clangd", pointing at a location
// inside an extracted archive) goes through version-placeholder resolution.
func resolveAssetBin(asset Asset, resolvedFile, version string) string {
	if asset.Bin == "" || asset.Bin == "{{source.asset.file}}" {
		return resolvedFile
	}
	return resolveVersionTemplate(asset.Bin, version)
}

// matchAsset picks the best asset for targets (most-specific-first, per
// platform.MasonTargetCandidates), falling back to a target-less
// ("universal") asset — the shape a single-binary-for-everyone package
// like phpactor's phar uses — only if no candidate target matches directly.
func matchAsset(assets []Asset, targets []string) (Asset, bool) {
	var universal *Asset
	for i := range assets {
		if assets[i].Targets == nil {
			universal = &assets[i]
		}
	}
	for _, want := range targets {
		for i := range assets {
			for _, t := range assets[i].Targets {
				if t == want {
					return assets[i], true
				}
			}
		}
	}
	if universal != nil {
		return *universal, true
	}
	return Asset{}, false
}

func installGitHubBinary(installDir string, spec PackageSpec, p purl, targets []string) (string, error) {
	owner, repo, ok := strings.Cut(p.Rest, "/")
	if !ok {
		return "", fmt.Errorf("lspmanager: github purl missing owner/repo: %q", spec.Source.ID)
	}
	asset, ok := matchAsset(spec.Source.Assets, targets)
	if !ok {
		return "", fmt.Errorf("lspmanager: %s has no release asset for this platform (tried %v)", spec.Name, targets)
	}

	file := resolveVersionTemplate(asset.File, p.Version)
	url := fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", owner, repo, p.Version, file)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("lspmanager: downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lspmanager: downloading %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("lspmanager: reading %s: %w", url, err)
	}

	binRel := resolveAssetBin(asset, file, p.Version)
	switch {
	case strings.HasSuffix(file, ".zip"):
		if err := extractZip(data, installDir); err != nil {
			return "", fmt.Errorf("lspmanager: extracting %s: %w", file, err)
		}
	case strings.HasSuffix(file, ".tar.gz"), strings.HasSuffix(file, ".tgz"):
		if err := extractTarGz(data, installDir); err != nil {
			return "", fmt.Errorf("lspmanager: extracting %s: %w", file, err)
		}
	default:
		// Raw binary, no archive — write it directly under its own name.
		if err := os.WriteFile(filepath.Join(installDir, file), data, 0o755); err != nil {
			return "", fmt.Errorf("lspmanager: writing %s: %w", file, err)
		}
	}

	return filepath.Join(installDir, filepath.FromSlash(binRel)), nil
}

// extractZip and extractTarGz both guard against zip-slip (an archive entry
// whose path escapes destDir via "..") — a real correctness/security
// concern for anything unpacking third-party archives, not a hypothetical.

func extractZip(data []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		target, err := safeJoin(destDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractOneZipFile(f, target); err != nil {
			return err
		}
	}
	return nil
}

func extractOneZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode()|0o200)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func extractTarGz(data []byte, destDir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)|0o200)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

// safeJoin joins destDir with an archive-provided relative path, rejecting
// any result that escapes destDir (a malicious or malformed archive entry
// using ".." to write outside the intended install directory).
func safeJoin(destDir, name string) (string, error) {
	target := filepath.Join(destDir, name)
	if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
		return "", fmt.Errorf("lspmanager: archive entry %q escapes destination directory", name)
	}
	return target, nil
}
