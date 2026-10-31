package lspclient

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// RPCError mirrors a JSON-RPC error object — returned from Call whenever
// the server responds with "error" instead of "result".
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("lspclient: rpc error %d: %s", e.Code, e.Message)
}

// Notification is a server-to-client message with no id — the only kind
// DaePlex's own feature set needs to react to today is
// textDocument/publishDiagnostics, but this stays generic rather than
// special-cased to just that one method.
type Notification struct {
	Method string
	Params json.RawMessage
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type pendingCall struct {
	result json.RawMessage
	err    error
}

// Client is a bare JSON-RPC 2.0 connection over two arbitrary streams (a
// subprocess's stdin/stdout in real use, an in-memory pipe in tests) — no
// LSP-specific knowledge lives here at all, that's lifecycle.go/sync.go's
// job. Client owns exactly two things: matching a response back to the
// Call that sent it, and handing every unsolicited server message
// (a notification, or a server-to-client request this client doesn't
// support) somewhere sane instead of silently dropping or hanging on it.
type Client struct {
	writer  *frameWriter
	writeMu sync.Mutex

	nextID int64

	mu       sync.Mutex
	pending  map[int64]chan pendingCall
	onNotify func(Notification)

	closed    chan struct{}
	closeErr  error
	closeOnce sync.Once
}

// NewClient starts the connection's read loop over r/w. Call Close (or let
// the underlying process exit, which ends r) to stop it.
func NewClient(r io.Reader, w io.Writer) *Client {
	c := &Client{
		writer:  &frameWriter{w: w},
		pending: make(map[int64]chan pendingCall),
		closed:  make(chan struct{}),
	}
	go c.readLoop(newFrameReader(r))
	return c
}

// OnNotification registers the single handler invoked (from the read
// loop's own goroutine — handlers must not block or call back into Client
// synchronously in a way that deadlocks) for every server-to-client
// message with no id. Must be called before any notification can arrive
// that the caller cares about catching — typically right after NewClient,
// before Initialize.
func (c *Client) OnNotification(handler func(Notification)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onNotify = handler
}

// Call sends a request and blocks for its response (or for Close/the
// connection dying, whichever comes first).
func (c *Client) Call(method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan pendingCall, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	msg := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", id, method, params}

	c.writeMu.Lock()
	err := c.writer.writeMessage(msg)
	c.writeMu.Unlock()
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("lspclient: sending %s: %w", method, err)
	}

	select {
	case res := <-ch:
		return res.result, res.err
	case <-c.closed:
		return nil, fmt.Errorf("lspclient: connection closed while waiting for %s response: %w", method, c.closeErr)
	}
}

// Notify sends a fire-and-forget message — no response is expected or
// waited for (textDocument/didOpen, didChange, initialized, exit).
func (c *Client) Notify(method string, params any) error {
	msg := struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", method, params}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := c.writer.writeMessage(msg); err != nil {
		return fmt.Errorf("lspclient: sending %s: %w", method, err)
	}
	return nil
}

// Close stops waiting on any in-flight Call (each returns an error) — it
// does not itself terminate a spawned server process; see
// lifecycle.go's Stop for the full shutdown/exit/kill sequence.
func (c *Client) Close(err error) {
	c.closeOnce.Do(func() {
		if err == nil {
			err = fmt.Errorf("lspclient: client closed")
		}
		c.closeErr = err
		close(c.closed)
	})
}

func (c *Client) readLoop(r *frameReader) {
	for {
		raw, err := r.readMessage()
		if err != nil {
			c.Close(err)
			c.failAllPending(err)
			return
		}

		var msg rpcMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue // one malformed frame shouldn't take down the whole connection
		}

		switch {
		case msg.Method != "" && len(msg.ID) == 0:
			c.dispatchNotification(Notification{Method: msg.Method, Params: msg.Params})
		case msg.Method != "":
			// A server-to-client *request* (has both a method and an id) —
			// none of DaePlex's own ~10 message list needs to serve
			// one today, but a real server is entitled to send one (e.g.
			// workspace/configuration) and will hang waiting for a reply
			// if we never answer. Respond "method not found" rather than
			// silently dropping it.
			c.replyMethodNotFound(msg.ID, msg.Method)
		default:
			c.deliverResponse(msg)
		}
	}
}

func (c *Client) dispatchNotification(n Notification) {
	c.mu.Lock()
	handler := c.onNotify
	c.mu.Unlock()
	if handler != nil {
		handler(n)
	}
}

func (c *Client) deliverResponse(msg rpcMessage) {
	var id int64
	if err := json.Unmarshal(msg.ID, &id); err != nil {
		return // response to an id we couldn't have sent (not int64) — ignore
	}

	c.mu.Lock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if !ok {
		return // response to a call we're no longer waiting on
	}

	if msg.Error != nil {
		ch <- pendingCall{err: msg.Error}
	} else {
		ch <- pendingCall{result: msg.Result}
	}
}

func (c *Client) replyMethodNotFound(id json.RawMessage, method string) {
	msg := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Error   RPCError        `json:"error"`
	}{"2.0", id, RPCError{Code: -32601, Message: fmt.Sprintf("method not supported by this client: %s", method)}}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.writer.writeMessage(msg) // best-effort — nothing more useful to do if this itself fails
}

func (c *Client) failAllPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		ch <- pendingCall{err: fmt.Errorf("lspclient: connection lost: %w", err)}
		delete(c.pending, id)
	}
}
