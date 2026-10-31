// Package lspclient is a minimal LSP client: enough of JSON-RPC 2.0's
// Content-Length-framed transport, request/response correlation, and the
// specific ~10 message types DaePlex's editor actually needs
// (initialize, textDocument/didOpen, didChange, hover, completion,
// formatting, publishDiagnostics, shutdown, exit) to drive a real language
// server. Deliberately not a full protocol library — every message shape
// here was confirmed against a real running gopls (2026-09-20) rather than
// typed from the spec blind, and the surface stays exactly as small as
// DaePlex's own feature set needs.
package lspclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// frameWriter serializes JSON-RPC messages with the Content-Length header
// framing LSP requires — confirmed against a real gopls exchange, not
// assumed from the spec text alone.
type frameWriter struct {
	w io.Writer
}

func (f *frameWriter) writeMessage(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("lspclient: marshaling message: %w", err)
	}
	if _, err := fmt.Fprintf(f.w, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return fmt.Errorf("lspclient: writing frame header: %w", err)
	}
	if _, err := f.w.Write(data); err != nil {
		return fmt.Errorf("lspclient: writing frame body: %w", err)
	}
	return nil
}

// frameReader reads one Content-Length-framed message at a time from a
// bufio.Reader. A real server (confirmed against gopls) sends only
// "Content-Length" — no "Content-Type" header — but readMessage tolerates
// and ignores any other header line rather than erroring on it, since nothing
// in the spec forbids one.
type frameReader struct {
	r *bufio.Reader
}

func newFrameReader(r io.Reader) *frameReader {
	return &frameReader{r: bufio.NewReader(r)}
}

func (f *frameReader) readMessage() (json.RawMessage, error) {
	var length int
	haveLength := false

	for {
		line, err := f.r.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("lspclient: reading frame header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // blank line ends the header block
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("lspclient: malformed Content-Length %q: %w", value, err)
			}
			length = n
			haveLength = true
		}
	}
	if !haveLength {
		return nil, fmt.Errorf("lspclient: frame had no Content-Length header")
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(f.r, body); err != nil {
		return nil, fmt.Errorf("lspclient: reading frame body: %w", err)
	}
	return json.RawMessage(body), nil
}
