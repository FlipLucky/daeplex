package langdetect

import (
	"testing"

	"github.com/fliplucky/daeplex/internal/types"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		path string
		want types.Language
	}{
		{"README.md", types.LanguageMarkdown},
		{"notes.markdown", types.LanguageMarkdown},
		{"NOTES.MD", types.LanguageMarkdown}, // case-insensitive
		{"index.html", types.LanguageHTML},
		{"page.htm", types.LanguageHTML},
		{"main.go", types.LanguageGo},
		{"data.json", types.LanguageJSON},
		{"app.js", types.LanguageJavaScript},
		{"app.mjs", types.LanguageJavaScript},
		{"app.cjs", types.LanguageJavaScript},
		{"app.ts", types.LanguageTypeScript},
		{"component.tsx", types.LanguageTSX},
		{"style.css", types.LanguageCSS},
		{"style.scss", types.LanguageSCSS},
		{"index.php", types.LanguagePHP},
		{"main.dart", types.LanguageDart},
		{"lib.c", types.LanguageC},
		{"lib.h", types.LanguageC},
		{"lib.cpp", types.LanguageCPP},
		{"lib.hpp", types.LanguageCPP},
		{"config.yaml", types.LanguageYAML},
		{"config.yml", types.LanguageYAML},
		{"Dockerfile", types.LanguageDockerfile},        // no extension at all — matched by base name
		{"dockerfile", types.LanguageDockerfile},        // case-insensitive
		{"Dockerfile.dev", types.LanguageDockerfile},    // multi-stage/variant naming convention
		{"", types.LanguagePlainText},                   // unsaved buffer, no path at all
		{"Makefile", types.LanguagePlainText},           // no extension, not a special-cased name
		{"archive.tar.gz", types.LanguagePlainText},     // unrecognized extension
		{"/a/b/c/README.md", types.LanguageMarkdown},    // extension detection ignores directory components
		{"/a/b/c/Dockerfile", types.LanguageDockerfile}, // base-name detection also ignores directory components
	}
	for _, c := range cases {
		if got := Detect(c.path); got != c.want {
			t.Errorf("Detect(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
