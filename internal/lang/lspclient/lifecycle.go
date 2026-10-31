package lspclient

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// Server wraps a Client with the subprocess it's actually talking to —
// Client alone has no notion of a spawned process, only two streams.
type Server struct {
	*Client
	cmd *exec.Cmd
}

// Start spawns binPath as a language server subprocess and wires a Client
// to its stdin/stdout. The server's stderr is discarded rather than
// captured — real servers (gopls included) log routine, high-volume
// diagnostic chatter there; surfacing it usefully is a real future
// improvement, not silently dropped by oversight, but out of scope for
// getting a first working client talking to a real server.
func Start(binPath string, args ...string) (*Server, error) {
	cmd := exec.Command(binPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lspclient: opening stdin pipe for %s: %w", binPath, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lspclient: opening stdout pipe for %s: %w", binPath, err)
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lspclient: starting %s: %w", binPath, err)
	}

	return &Server{Client: NewClient(stdout, stdin), cmd: cmd}, nil
}

// CompletionOptions is the subset of a server's advertised completion
// capability DaePlex's own trigger-character handling needs.
type CompletionOptions struct {
	TriggerCharacters []string `json:"triggerCharacters"`
}

// Capabilities is the subset of InitializeResult.capabilities DaePlex
// actually reads — every field name and shape here was confirmed against a
// real gopls initialize response (2026-09-20), not typed from the spec
// blind. HoverProvider/DocumentFormattingProvider are deliberately
// json.RawMessage rather than bool: per spec a provider capability can be
// either a plain boolean or a options object (e.g. {"workDoneProgress":
// true}), and gopls itself sends different shapes for different
// capabilities in the same response — Supported() below normalizes both.
type Capabilities struct {
	HoverProvider              json.RawMessage        `json:"hoverProvider"`
	DocumentFormattingProvider json.RawMessage        `json:"documentFormattingProvider"`
	CompletionProvider         *CompletionOptions     `json:"completionProvider"`
	SemanticTokensProvider     *SemanticTokensOptions `json:"semanticTokensProvider"`
	SignatureHelpProvider      *SignatureHelpOptions  `json:"signatureHelpProvider"`
	// PositionEncoding is empty whenever a server (gopls included, as of
	// this writing) doesn't negotiate LSP 3.17's positionEncoding
	// extension — per spec, absence means UTF-16, never "unspecified."
	PositionEncoding string `json:"positionEncoding"`
}

// Supported reports whether a provider-capability field (HoverProvider,
// DocumentFormattingProvider) indicates the feature is available — true,
// or any non-null object; false or absent means not supported.
func Supported(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	s := string(raw)
	return s != "null" && s != "false"
}

// UsesUTF16Positions reports whether Position.Character should be
// interpreted as UTF-16 code units for this server — true whenever
// PositionEncoding is empty (the spec default) or explicitly "utf-16".
func (c Capabilities) UsesUTF16Positions() bool {
	return c.PositionEncoding == "" || c.PositionEncoding == "utf-16"
}

type initializeResult struct {
	Capabilities Capabilities `json:"capabilities"`
}

// Initialize performs the full initialize/initialized handshake — the
// mandatory first exchange before any other request is valid per spec —
// and returns the server's real advertised capabilities. rootURI is a
// "file://" URI; DaePlex's per-buffer model means this is usually the
// buffer's own directory, not a real multi-root workspace.
func (s *Server) Initialize(rootURI string) (Capabilities, error) {
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		// gopls specifically (confirmed empirically, v0.23.0) doesn't
		// advertise semanticTokensProvider at all unless this gopls-
		// specific setting is turned on — per spec, initializationOptions
		// is entirely server-defined/opaque, so a server that doesn't
		// recognize this key just ignores it.
		"initializationOptions": map[string]any{"semanticTokens": true},
		"capabilities": map[string]any{
			"general": map[string]any{
				"positionEncodings": []string{"utf-8", "utf-16"},
			},
			"textDocument": map[string]any{
				"hover":              map[string]any{"contentFormat": []string{"plaintext", "markdown"}},
				"completion":         map[string]any{"completionItem": map[string]any{"snippetSupport": false}},
				"publishDiagnostics": map[string]any{},
				// semanticTokens (T48): the generous, standard LSP token-
				// type/modifier vocabulary (the same set VS Code itself
				// declares) — DaePlex only maps a handful of these to
				// a real types.Style (see editor/semantictokens.go), but
				// declaring the full set here just tells the server what
				// it's allowed to send, not what gets rendered.
				"semanticTokens": map[string]any{
					"requests": map[string]any{"full": true},
					"tokenTypes": []string{
						"namespace", "type", "class", "enum", "interface",
						"struct", "typeParameter", "parameter", "variable",
						"property", "enumMember", "event", "function",
						"method", "macro", "keyword", "modifier", "comment",
						"string", "number", "regexp", "operator", "decorator",
					},
					"tokenModifiers": []string{
						"declaration", "definition", "readonly", "static",
						"deprecated", "abstract", "async", "modification",
						"documentation", "defaultLibrary",
					},
					"formats": []string{"relative"},
				},
				"signatureHelp": map[string]any{
					"signatureInformation": map[string]any{
						"documentationFormat": []string{"plaintext", "markdown"},
					},
				},
			},
		},
	}

	raw, err := s.Call("initialize", params)
	if err != nil {
		return Capabilities{}, fmt.Errorf("lspclient: initialize: %w", err)
	}
	var result initializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return Capabilities{}, fmt.Errorf("lspclient: decoding initialize result: %w", err)
	}

	if err := s.Notify("initialized", struct{}{}); err != nil {
		return Capabilities{}, fmt.Errorf("lspclient: initialized: %w", err)
	}
	return result.Capabilities, nil
}

// Stop performs the spec-mandated shutdown/exit sequence, then waits for
// the process to actually exit — killing it after a bounded timeout if it
// doesn't, since a hung or misbehaving server must never leave DaePlex
// waiting forever to close a buffer or quit.
func (s *Server) Stop() error {
	_, shutdownErr := s.Call("shutdown", nil)
	_ = s.Notify("exit", nil)

	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()

	select {
	case waitErr := <-done:
		if shutdownErr != nil {
			return shutdownErr
		}
		return waitErr
	case <-time.After(3 * time.Second):
		_ = s.cmd.Process.Kill()
		<-done
		return fmt.Errorf("lspclient: server did not exit after shutdown/exit within 3s, killed")
	}
}
