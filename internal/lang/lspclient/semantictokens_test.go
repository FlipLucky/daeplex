package lspclient

import (
	"reflect"
	"testing"
)

// TestDecodeSemanticTokensFlatDeltas covers the two deltaLine cases spelled
// out in the spec: deltaLine==0 means deltaStartChar is relative to the
// previous token's own column; any other deltaLine resets the column to an
// absolute value.
func TestDecodeSemanticTokensFlatDeltas(t *testing.T) {
	// Token 1: line 0, char 5, length 3, type 2, modifiers 0.
	// Token 2: same line (deltaLine=0), char 5+4=9, length 2, type 1, mods 1.
	// Token 3: deltaLine=2 -> line 2, char reset to 1 (absolute), length 4, type 0, mods 0.
	data := []uint32{
		0, 5, 3, 2, 0,
		0, 4, 2, 1, 1,
		2, 1, 4, 0, 0,
	}
	got := DecodeSemanticTokens(data)
	want := []SemanticToken{
		{Line: 0, StartChar: 5, Length: 3, TokenType: 2, Modifiers: 0},
		{Line: 0, StartChar: 9, Length: 2, TokenType: 1, Modifiers: 1},
		{Line: 2, StartChar: 1, Length: 4, TokenType: 0, Modifiers: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DecodeSemanticTokens() = %+v, want %+v", got, want)
	}
}

func TestDecodeSemanticTokensEmpty(t *testing.T) {
	if got := DecodeSemanticTokens(nil); len(got) != 0 {
		t.Errorf("DecodeSemanticTokens(nil) = %+v, want empty", got)
	}
}

// TestDecodeSemanticTokensIgnoresTrailingPartialGroup covers a
// malformed/truncated response (not a multiple of 5) — must not panic, and
// must still decode every complete group before the partial one.
func TestDecodeSemanticTokensIgnoresTrailingPartialGroup(t *testing.T) {
	data := []uint32{0, 0, 3, 1, 0, 0, 1} // one full group + 2 stray ints
	got := DecodeSemanticTokens(data)
	want := []SemanticToken{{Line: 0, StartChar: 0, Length: 3, TokenType: 1, Modifiers: 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DecodeSemanticTokens() = %+v, want %+v", got, want)
	}
}
