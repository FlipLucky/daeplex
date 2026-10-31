package lspmanager

import "testing"

func TestLoadIndexEmptyWhenNoFileExists(t *testing.T) {
	idx, err := LoadIndex(t.TempDir())
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	if len(idx) != 0 {
		t.Errorf("LoadIndex() on a fresh dir = %v, want empty", idx)
	}
}

func TestRecordInstalledThenLoadIndexRoundTrips(t *testing.T) {
	dir := t.TempDir()
	installed := InstalledServer{PackageName: "gopls", BinName: "gopls", BinPath: "/bin/gopls", Version: "v0.23.0"}

	if err := RecordInstalled(dir, installed); err != nil {
		t.Fatalf("RecordInstalled() error = %v", err)
	}
	idx, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("LoadIndex() error = %v", err)
	}
	got, ok := idx["gopls"]
	if !ok {
		t.Fatal("LoadIndex() missing the recorded gopls entry")
	}
	if got.BinPath != installed.BinPath || got.Version != installed.Version {
		t.Errorf("LoadIndex() entry = %+v, want it to match RecordInstalled's input", got)
	}
}

func TestRecordInstalledOverwritesPriorVersion(t *testing.T) {
	dir := t.TempDir()
	_ = RecordInstalled(dir, InstalledServer{PackageName: "gopls", Version: "v0.22.0"})
	_ = RecordInstalled(dir, InstalledServer{PackageName: "gopls", Version: "v0.23.0"})

	idx, _ := LoadIndex(dir)
	if idx["gopls"].Version != "v0.23.0" {
		t.Errorf("idx[gopls].Version = %q, want the newer v0.23.0 (upgrade should replace, not duplicate)", idx["gopls"].Version)
	}
}

func TestRemoveInstalledDeletesEntry(t *testing.T) {
	dir := t.TempDir()
	_ = RecordInstalled(dir, InstalledServer{PackageName: "gopls", Version: "v0.23.0"})

	if err := RemoveInstalled(dir, "gopls"); err != nil {
		t.Fatalf("RemoveInstalled() error = %v", err)
	}
	idx, _ := LoadIndex(dir)
	if _, ok := idx["gopls"]; ok {
		t.Error("gopls still present after RemoveInstalled()")
	}
}

func TestRemoveInstalledOnUnknownPackageIsNotAnError(t *testing.T) {
	if err := RemoveInstalled(t.TempDir(), "never-installed"); err != nil {
		t.Errorf("RemoveInstalled() on an unknown package error = %v, want nil", err)
	}
}
