package lspclient

import (
	"encoding/json"
	"io"
	"testing"
	"time"
)

// fakeServer speaks the same frame protocol as a real language server,
// entirely in-memory over io.Pipe — lets Client's correlation/dispatch
// logic be tested deterministically without spawning a real process.
type fakeServer struct {
	reader *frameReader
	writer *frameWriter
	// received records every message this fake server read, for tests to
	// assert against.
	received chan rpcMessage
}

func newFakeServerPair(t *testing.T) (*Client, *fakeServer) {
	t.Helper()
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()

	client := NewClient(serverToClientR, clientToServerW)
	fs := &fakeServer{
		reader:   newFrameReader(clientToServerR),
		writer:   &frameWriter{w: serverToClientW},
		received: make(chan rpcMessage, 16),
	}
	go fs.readLoop()
	t.Cleanup(func() {
		_ = clientToServerW.Close()
		_ = serverToClientW.Close()
	})
	return client, fs
}

func (fs *fakeServer) readLoop() {
	for {
		raw, err := fs.reader.readMessage()
		if err != nil {
			return
		}
		var msg rpcMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		fs.received <- msg
	}
}

func (fs *fakeServer) waitForMessage(t *testing.T, method string) rpcMessage {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-fs.received:
			if msg.Method == method || method == "" {
				return msg
			}
		case <-deadline:
			t.Fatalf("timed out waiting for a %q message", method)
		}
	}
}

func (fs *fakeServer) respondResult(id json.RawMessage, result any) {
	data, _ := json.Marshal(result)
	fs.writer.writeMessage(rpcMessage{JSONRPC: "2.0", ID: id, Result: data})
}

func (fs *fakeServer) respondError(id json.RawMessage, code int, message string) {
	fs.writer.writeMessage(rpcMessage{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: message}})
}

func (fs *fakeServer) sendNotification(method string, params any) {
	data, _ := json.Marshal(params)
	fs.writer.writeMessage(rpcMessage{JSONRPC: "2.0", Method: method, Params: data})
}

func (fs *fakeServer) sendRequest(id int, method string) {
	fs.writer.writeMessage(rpcMessage{JSONRPC: "2.0", ID: json.RawMessage(mustJSON(id)), Method: method})
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

func TestCallRoundTrip(t *testing.T) {
	client, fs := newFakeServerPair(t)

	resultCh := make(chan json.RawMessage, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := client.Call("foo/bar", map[string]int{"x": 1})
		resultCh <- res
		errCh <- err
	}()

	req := fs.waitForMessage(t, "foo/bar")
	if req.Method != "foo/bar" {
		t.Fatalf("server received method %q, want foo/bar", req.Method)
	}
	fs.respondResult(req.ID, map[string]string{"ok": "yes"})

	if err := <-errCh; err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(<-resultCh, &got); err != nil {
		t.Fatalf("unmarshaling result: %v", err)
	}
	if got["ok"] != "yes" {
		t.Errorf("result = %v, want {ok: yes}", got)
	}
}

func TestCallReturnsRPCError(t *testing.T) {
	client, fs := newFakeServerPair(t)

	errCh := make(chan error, 1)
	go func() {
		_, err := client.Call("will/fail", nil)
		errCh <- err
	}()

	req := fs.waitForMessage(t, "will/fail")
	fs.respondError(req.ID, -32602, "invalid params")

	err := <-errCh
	if err == nil {
		t.Fatal("Call() error = nil, want an RPCError")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("Call() error type = %T, want *RPCError", err)
	}
	if rpcErr.Code != -32602 {
		t.Errorf("RPCError.Code = %d, want -32602", rpcErr.Code)
	}
}

func TestNotifyDoesNotWaitForResponse(t *testing.T) {
	client, fs := newFakeServerPair(t)

	done := make(chan error, 1)
	go func() { done <- client.Notify("textDocument/didOpen", map[string]int{"v": 1}) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Notify() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Notify() blocked — it must never wait for a response")
	}

	msg := fs.waitForMessage(t, "textDocument/didOpen")
	if len(msg.ID) != 0 {
		t.Errorf("notification carried an id (%s) — notifications must have none", msg.ID)
	}
}

func TestNotificationDispatchedToHandler(t *testing.T) {
	client, fs := newFakeServerPair(t)

	received := make(chan Notification, 1)
	client.OnNotification(func(n Notification) { received <- n })

	fs.sendNotification("textDocument/publishDiagnostics", map[string]string{"uri": "file:///x.go"})

	select {
	case n := <-received:
		if n.Method != "textDocument/publishDiagnostics" {
			t.Errorf("Notification.Method = %q, want textDocument/publishDiagnostics", n.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the notification handler to fire")
	}
}

func TestServerToClientRequestGetsMethodNotFound(t *testing.T) {
	_, fs := newFakeServerPair(t)

	fs.sendRequest(99, "workspace/configuration")

	resp := fs.waitForMessage(t, "") // the reply has no "method", just id+error
	var id int
	_ = json.Unmarshal(resp.ID, &id)
	if id != 99 {
		t.Fatalf("reply id = %d, want 99", id)
	}
	if resp.Error == nil {
		t.Fatal("expected an error reply for an unsupported server-to-client request, got none")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("reply error code = %d, want -32601 (method not found)", resp.Error.Code)
	}
}

func TestCloseFailsPendingCallsPromptly(t *testing.T) {
	client, fs := newFakeServerPair(t)

	errCh := make(chan error, 1)
	go func() {
		_, err := client.Call("never/answered", nil)
		errCh <- err
	}()
	fs.waitForMessage(t, "never/answered")

	client.Close(nil)

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Call() error = nil after Close(), want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Call() did not return promptly after Close()")
	}
}

func TestConnectionEOFFailsPendingCalls(t *testing.T) {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()
	client := NewClient(serverToClientR, clientToServerW)
	// Drain whatever the client writes — an unbuffered io.Pipe write blocks
	// until something reads it, and without this, Call()'s own write would
	// block forever before ever reaching the point where it'd notice the
	// connection died, regardless of what happens to serverToClientW below.
	go io.Copy(io.Discard, clientToServerR)

	errCh := make(chan error, 1)
	go func() {
		_, err := client.Call("never/answered", nil)
		errCh <- err
	}()

	// Simulate the server process exiting: close its write end, giving the
	// client's read loop an EOF.
	_ = serverToClientW.Close()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Call() error = nil after the connection died, want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Call() did not return after the connection died")
	}
	_ = clientToServerW.Close()
}
