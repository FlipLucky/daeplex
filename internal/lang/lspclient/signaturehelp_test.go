package lspclient

import "testing"

func TestActiveLabelReturnsActiveSignature(t *testing.T) {
	r := SignatureHelpResult{
		Signatures: []SignatureInformation{
			{Label: "func A()"},
			{Label: "func B(x int)"},
		},
		ActiveSignature: 1,
	}
	if got, want := r.ActiveLabel(), "func B(x int)"; got != want {
		t.Errorf("ActiveLabel() = %q, want %q", got, want)
	}
}

func TestActiveLabelEmptyWhenNoSignatures(t *testing.T) {
	if got := (SignatureHelpResult{}).ActiveLabel(); got != "" {
		t.Errorf("ActiveLabel() = %q, want empty", got)
	}
}

// TestActiveLabelFallsBackOnOutOfRangeIndex covers a malformed response
// (ActiveSignature pointing past the end of Signatures) — must fall back
// to the first signature, not panic or silently return "".
func TestActiveLabelFallsBackOnOutOfRangeIndex(t *testing.T) {
	r := SignatureHelpResult{
		Signatures:      []SignatureInformation{{Label: "func A()"}},
		ActiveSignature: 7,
	}
	if got, want := r.ActiveLabel(), "func A()"; got != want {
		t.Errorf("ActiveLabel() = %q, want %q", got, want)
	}
}
