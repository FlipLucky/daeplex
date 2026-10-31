package tuibase

import (
	"reflect"
	"testing"
)

func TestWrapTextShortLineFitsOnOneLine(t *testing.T) {
	got := wrapText("hello world", 40)
	want := []string{"hello world"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextBreaksOnWordBoundaries(t *testing.T) {
	got := wrapText("the quick brown fox jumps", 10)
	want := []string{"the quick", "brown fox", "jumps"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextPreservesExistingNewlinesAsParagraphs(t *testing.T) {
	got := wrapText("line one\nline two", 40)
	want := []string{"line one", "line two"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextPreservesBlankLineBetweenParagraphs(t *testing.T) {
	got := wrapText("a\n\nb", 40)
	want := []string{"a", "", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextZeroOrNegativeWidthReturnsTextUnwrapped(t *testing.T) {
	got := wrapText("hello", 0)
	want := []string{"hello"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}

func TestWrapTextSingleWordLongerThanWidthIsNotSplit(t *testing.T) {
	// A word longer than the wrap width still goes on its own line whole —
	// this isn't a general text-layout algorithm that hyphenates.
	got := wrapText("supercalifragilisticexpialidocious", 10)
	want := []string{"supercalifragilisticexpialidocious"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapText() = %v, want %v", got, want)
	}
}
