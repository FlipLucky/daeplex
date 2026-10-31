package lspclient

import (
	"encoding/json"
	"testing"
)

func TestExtractMarkupTextFromMarkupContent(t *testing.T) {
	raw := json.RawMessage(`{"kind":"plaintext","value":"func foo()"}`)
	if got := extractMarkupText(raw); got != "func foo()" {
		t.Errorf("extractMarkupText() = %q, want %q", got, "func foo()")
	}
}

func TestExtractMarkupTextFromPlainString(t *testing.T) {
	raw := json.RawMessage(`"just a string"`)
	if got := extractMarkupText(raw); got != "just a string" {
		t.Errorf("extractMarkupText() = %q, want %q", got, "just a string")
	}
}

func TestExtractMarkupTextEmpty(t *testing.T) {
	if got := extractMarkupText(nil); got != "" {
		t.Errorf("extractMarkupText(nil) = %q, want empty", got)
	}
}

func TestParseCompletionResultObjectShape(t *testing.T) {
	raw := json.RawMessage(`{"isIncomplete":true,"items":[{"label":"Print"},{"label":"Printf"}]}`)
	list, err := parseCompletionResult(raw)
	if err != nil {
		t.Fatalf("parseCompletionResult() error = %v", err)
	}
	if !list.IsIncomplete || len(list.Items) != 2 {
		t.Errorf("parseCompletionResult() = %+v, want IsIncomplete=true and 2 items", list)
	}
}

func TestParseCompletionResultBareArrayShape(t *testing.T) {
	raw := json.RawMessage(`[{"label":"foo"},{"label":"bar"}]`)
	list, err := parseCompletionResult(raw)
	if err != nil {
		t.Fatalf("parseCompletionResult() error = %v", err)
	}
	if len(list.Items) != 2 || list.Items[0].Label != "foo" {
		t.Errorf("parseCompletionResult() = %+v, want 2 items starting with foo", list)
	}
}

func TestParseCompletionResultNull(t *testing.T) {
	list, err := parseCompletionResult(json.RawMessage(`null`))
	if err != nil {
		t.Fatalf("parseCompletionResult() error = %v", err)
	}
	if len(list.Items) != 0 {
		t.Errorf("parseCompletionResult(null) = %+v, want no items", list)
	}
}

func TestCompletionItemDocumentationTextHandlesBothShapes(t *testing.T) {
	item := CompletionItem{Documentation: json.RawMessage(`{"kind":"markdown","value":"docs here"}`)}
	if got := item.DocumentationText(); got != "docs here" {
		t.Errorf("DocumentationText() = %q, want %q", got, "docs here")
	}
	item2 := CompletionItem{Documentation: json.RawMessage(`"plain docs"`)}
	if got := item2.DocumentationText(); got != "plain docs" {
		t.Errorf("DocumentationText() = %q, want %q", got, "plain docs")
	}
}

func TestSupportedTrueFalseObjectNull(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{`true`, true},
		{`false`, false},
		{`{"workDoneProgress":true}`, true},
		{`null`, false},
		{``, false},
	}
	for _, c := range cases {
		var raw json.RawMessage
		if c.raw != "" {
			raw = json.RawMessage(c.raw)
		}
		if got := Supported(raw); got != c.want {
			t.Errorf("Supported(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestCapabilitiesUsesUTF16PositionsDefaultAndExplicit(t *testing.T) {
	if !(Capabilities{}).UsesUTF16Positions() {
		t.Error("Capabilities{} (no positionEncoding sent) should default to UTF-16 per spec")
	}
	if !(Capabilities{PositionEncoding: "utf-16"}).UsesUTF16Positions() {
		t.Error("explicit utf-16 should report true")
	}
	if (Capabilities{PositionEncoding: "utf-8"}).UsesUTF16Positions() {
		t.Error("explicit utf-8 should report false")
	}
}

func TestParsePublishDiagnostics(t *testing.T) {
	raw := json.RawMessage(`{"uri":"file:///x.go","version":3,"diagnostics":[{"range":{"start":{"line":1,"character":2},"end":{"line":1,"character":5}},"severity":1,"message":"undefined: foo"}]}`)
	params, err := ParsePublishDiagnostics(raw)
	if err != nil {
		t.Fatalf("ParsePublishDiagnostics() error = %v", err)
	}
	if params.URI != "file:///x.go" || params.Version != 3 || len(params.Diagnostics) != 1 {
		t.Fatalf("ParsePublishDiagnostics() = %+v, want uri/version/1 diagnostic", params)
	}
	if params.Diagnostics[0].Severity != SeverityError {
		t.Errorf("Diagnostics[0].Severity = %d, want SeverityError", params.Diagnostics[0].Severity)
	}
}
