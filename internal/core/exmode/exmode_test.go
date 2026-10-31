package exmode

import (
	"reflect"
	"testing"
)

func TestParseWithLeadingColon(t *testing.T) {
	got := Parse(":w somefile.go")
	want := Command{Name: "w", Args: []string{"somefile.go"}, Raw: "somefile.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(:w somefile.go) = %+v, want %+v", got, want)
	}
}

func TestParseWithoutLeadingColon(t *testing.T) {
	got := Parse("w somefile.go")
	want := Command{Name: "w", Args: []string{"somefile.go"}, Raw: "somefile.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(w somefile.go) = %+v, want %+v", got, want)
	}
}

func TestParseNoArgs(t *testing.T) {
	got := Parse(":q")
	want := Command{Name: "q", Args: []string{}}
	if got.Name != want.Name || len(got.Args) != 0 {
		t.Errorf("Parse(:q) = %+v, want %+v", got, want)
	}
}

func TestParseMultipleArgs(t *testing.T) {
	got := Parse(":LspInstall go extra")
	want := Command{Name: "LspInstall", Args: []string{"go", "extra"}, Raw: "go extra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(:LspInstall go extra) = %+v, want %+v", got, want)
	}
}

func TestParseEmptyIsZeroValue(t *testing.T) {
	cases := []string{"", ":", "   ", " : "}
	for _, c := range cases {
		got := Parse(c)
		if got.Name != "" || len(got.Args) != 0 {
			t.Errorf("Parse(%q) = %+v, want the zero Command", c, got)
		}
	}
}

func TestParseTrimsWhitespace(t *testing.T) {
	got := Parse("  :w   somefile.go  ")
	want := Command{Name: "w", Args: []string{"somefile.go"}, Raw: "somefile.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse with extra whitespace = %+v, want %+v", got, want)
	}
}

func TestParsePreservesCaseAndBangSuffix(t *testing.T) {
	// "q!" is its own distinct command name today, not "q" plus a flag —
	// confirm parsing doesn't split or normalize it.
	got := Parse(":q!")
	if got.Name != "q!" {
		t.Errorf("Parse(:q!) Name = %q, want %q", got.Name, "q!")
	}
}

func TestParseSubstituteNoSpaceNeeded(t *testing.T) {
	// :s/foo/bar/g has no space between "s" and its argument — the name
	// scanner must stop at "/" the same way it stops at whitespace, unlike
	// every other command (:w, :LspInstall, ...) which always has a space.
	got := Parse(":s/foo/bar/g")
	want := Command{Name: "s", Args: []string{"/foo/bar/g"}, Raw: "/foo/bar/g"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(:s/foo/bar/g) = %+v, want %+v", got, want)
	}
}

func TestParseWholeBufferPrefix(t *testing.T) {
	got := Parse(":%s/foo/bar/g")
	want := Command{Name: "s", Args: []string{"/foo/bar/g"}, Raw: "/foo/bar/g", WholeBuffer: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse(:%%s/foo/bar/g) = %+v, want %+v", got, want)
	}
}

func TestParseSubstituteWithSpacesInSegments(t *testing.T) {
	// Confirms Raw (unlike Args) isn't whitespace-tokenized — a pattern or
	// replacement containing a space must survive intact.
	got := Parse(":s/hello world/goodbye world/")
	if got.Raw != "/hello world/goodbye world/" {
		t.Errorf("Raw = %q, want %q", got.Raw, "/hello world/goodbye world/")
	}
}

func TestParseSubstitute(t *testing.T) {
	cases := []struct {
		raw         string
		wantPattern string
		wantRepl    string
		wantFlags   string
		wantOK      bool
	}{
		{"/foo/bar/g", "foo", "bar", "g", true},
		{"/foo/bar/", "foo", "bar", "", true},
		{"/foo/bar", "foo", "bar", "", true},
		{"#a/b#c#", "a/b", "c", "", true},  // a different delimiter
		{`/a\/b/c/`, "a/b", "c", "", true}, // escaped delimiter in pattern
		{"/foo", "", "", "", false},        // no closing delimiter at all
		{"", "", "", "", false},            // empty
		{"foo/bar/", "", "", "", false},    // doesn't start with a delimiter
	}
	for _, c := range cases {
		got, ok := ParseSubstitute(c.raw)
		if ok != c.wantOK {
			t.Errorf("ParseSubstitute(%q) ok = %v, want %v", c.raw, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if got.Pattern != c.wantPattern || got.Replacement != c.wantRepl || got.Flags != c.wantFlags {
			t.Errorf("ParseSubstitute(%q) = %+v, want {Pattern:%q Replacement:%q Flags:%q}",
				c.raw, got, c.wantPattern, c.wantRepl, c.wantFlags)
		}
	}
}
