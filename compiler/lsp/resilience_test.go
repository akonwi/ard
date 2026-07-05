package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
)

func newTestRequest(t *testing.T, method string) jsonrpc2.Request {
	t.Helper()
	req, err := jsonrpc2.NewCall(jsonrpc2.NewNumberID(1), method, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

// TestConcurrentHandlerRecoversPanics verifies a panicking feature handler
// yields an error reply instead of crashing the process.
func TestConcurrentHandlerRecoversPanics(t *testing.T) {
	s := NewServer()
	s.requestTimeout = time.Second

	var mu sync.Mutex
	var replyErr error
	replied := make(chan struct{})

	handler := s.concurrentRequestHandler(func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		panic("boom")
	})

	req := newTestRequest(t, "textDocument/hover")
	err := handler(context.Background(), func(ctx context.Context, result interface{}, err error) error {
		mu.Lock()
		replyErr = err
		mu.Unlock()
		close(replied)
		return nil
	}, req)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-replied:
	case <-time.After(2 * time.Second):
		t.Fatal("no reply after panic")
	}
	mu.Lock()
	defer mu.Unlock()
	if replyErr == nil || !strings.Contains(replyErr.Error(), "internal server error") {
		t.Fatalf("expected internal error reply, got %v", replyErr)
	}
}

// TestConcurrentHandlerWatchdogTimesOut verifies a hung handler is cancelled
// and replies with a timeout error.
func TestConcurrentHandlerWatchdogTimesOut(t *testing.T) {
	s := NewServer()
	s.requestTimeout = 50 * time.Millisecond

	var mu sync.Mutex
	var replyErr error
	replied := make(chan struct{})

	handler := s.concurrentRequestHandler(func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		<-ctx.Done() // simulate a hung analysis that at least honors ctx
		time.Sleep(10 * time.Millisecond)
		return reply(ctx, "late", nil)
	})

	req := newTestRequest(t, "textDocument/references")
	err := handler(context.Background(), func(ctx context.Context, result interface{}, err error) error {
		mu.Lock()
		if replyErr == nil && err != nil {
			replyErr = err
		}
		mu.Unlock()
		select {
		case <-replied:
		default:
			close(replied)
		}
		return nil
	}, req)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-replied:
	case <-time.After(2 * time.Second):
		t.Fatal("no reply after timeout")
	}
	mu.Lock()
	defer mu.Unlock()
	if replyErr == nil || !strings.Contains(replyErr.Error(), "timed out") {
		t.Fatalf("expected timeout reply, got %v", replyErr)
	}
}

// TestConcurrentHandlerSingleReply verifies the guarded replier only replies
// once even when both the handler and the watchdog race to reply.
func TestConcurrentHandlerSingleReply(t *testing.T) {
	s := NewServer()
	s.requestTimeout = 30 * time.Millisecond

	var mu sync.Mutex
	replies := 0
	done := make(chan struct{})

	handler := s.concurrentRequestHandler(func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		time.Sleep(60 * time.Millisecond) // finishes after the watchdog fires
		return reply(ctx, "late", nil)
	})

	req := newTestRequest(t, "textDocument/completion")
	err := handler(context.Background(), func(ctx context.Context, result interface{}, err error) error {
		mu.Lock()
		replies++
		mu.Unlock()
		return nil
	}, req)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(200 * time.Millisecond)
		close(done)
	}()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if replies != 1 {
		t.Fatalf("expected exactly one reply, got %d", replies)
	}
}

// TestConcurrentHandlerNoDoubleReplyWithoutWatchdog verifies the reply-once
// guarantee holds when the watchdog is disabled and the handler panics after
// replying.
func TestConcurrentHandlerNoDoubleReplyWithoutWatchdog(t *testing.T) {
	s := NewServer()
	s.requestTimeout = 0

	var mu sync.Mutex
	replies := 0
	done := make(chan struct{})

	handler := s.concurrentRequestHandler(func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		_ = reply(ctx, "ok", nil)
		defer close(done)
		panic("after reply")
	})

	req := newTestRequest(t, "textDocument/hover")
	err := handler(context.Background(), func(ctx context.Context, result interface{}, err error) error {
		mu.Lock()
		replies++
		mu.Unlock()
		return nil
	}, req)
	if err != nil {
		t.Fatal(err)
	}

	<-done
	time.Sleep(50 * time.Millisecond) // allow the recover path to run
	mu.Lock()
	defer mu.Unlock()
	if replies != 1 {
		t.Fatalf("expected exactly one reply, got %d", replies)
	}
}

// TestConcurrentHandlerClientCancellation verifies parent-context
// cancellation replies with the LSP cancellation error, not a timeout.
func TestConcurrentHandlerClientCancellation(t *testing.T) {
	s := NewServer()
	s.requestTimeout = 5 * time.Second

	var mu sync.Mutex
	var replyErr error
	replied := make(chan struct{})

	handler := s.concurrentRequestHandler(func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		<-ctx.Done()
		return reply(ctx, nil, ctx.Err())
	})

	ctx, cancel := context.WithCancel(context.Background())
	req := newTestRequest(t, "textDocument/references")
	err := handler(ctx, func(ctx context.Context, result interface{}, err error) error {
		mu.Lock()
		replyErr = err
		mu.Unlock()
		close(replied)
		return nil
	}, req)
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	select {
	case <-replied:
	case <-time.After(2 * time.Second):
		t.Fatal("no reply after cancellation")
	}
	mu.Lock()
	defer mu.Unlock()
	if replyErr == nil || strings.Contains(replyErr.Error(), "timed out") {
		t.Fatalf("expected cancellation error, got %v", replyErr)
	}
}

// fakeStream feeds scripted read results to resilientStream.
type fakeStream struct {
	reads []fakeRead
	idx   int
}

type fakeRead struct {
	msg jsonrpc2.Message
	err error
}

func (f *fakeStream) Read(ctx context.Context) (jsonrpc2.Message, int64, error) {
	if f.idx >= len(f.reads) {
		return nil, 0, errFakeEOF
	}
	r := f.reads[f.idx]
	f.idx++
	return r.msg, 0, r.err
}

func (f *fakeStream) Write(ctx context.Context, msg jsonrpc2.Message) (int64, error) {
	return 0, nil
}

func (f *fakeStream) Close() error { return nil }

var errFakeEOF = errors.New("fake eof")

// TestResilientStreamSkipsMalformedBodies verifies decode errors are skipped
// while framing/IO errors stay fatal.
func TestResilientStreamSkipsMalformedBodies(t *testing.T) {
	good, err := jsonrpc2.NewNotification("initialized", nil)
	if err != nil {
		t.Fatal(err)
	}
	stream := resilientStream{inner: &fakeStream{reads: []fakeRead{
		{err: errors.New(`unmarshaling jsonrpc message: json: invalid`)},
		{msg: good},
		{err: errors.New(`failed reading header line: broken`)},
	}}}

	msg, _, err := stream.Read(context.Background())
	if err != nil {
		t.Fatalf("expected malformed body to be skipped, got error: %v", err)
	}
	if msg == nil || msg.(jsonrpc2.Request).Method() != "initialized" {
		t.Fatalf("expected the following good message, got %#v", msg)
	}

	// The header error is a desync: fatal.
	_, _, err = stream.Read(context.Background())
	if err == nil || !strings.Contains(err.Error(), "header") {
		t.Fatalf("expected fatal header error, got %v", err)
	}
}

// TestDispatchNeverPropagatesErrors: handler errors must not reach the
// jsonrpc2 connection, which would kill the whole session.
func TestDispatchNeverPropagatesErrors(t *testing.T) {
	s := NewServer()
	s.handlers["test/explode"] = func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		return errors.New("handler failure")
	}
	s.handlers["test/panicAfterReply"] = func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		_ = reply(ctx, "ok", nil)
		panic("after reply")
	}

	for _, method := range []string{"test/explode", "test/panicAfterReply", "no/such/method"} {
		req := newTestRequest(t, method)
		err := s.dispatch(context.Background(), func(ctx context.Context, result interface{}, err error) error {
			return nil
		}, req)
		if err != nil {
			t.Fatalf("dispatch(%s) returned %v; must always return nil", method, err)
		}
	}
}

// pipeRWC adapts an io.Pipe pair into the ReadWriteCloser the framed stream
// wants, so resilientStream can be tested against the real jsonrpc2 stream.
type pipeRWC struct {
	*io.PipeReader
	*io.PipeWriter
}

func (p pipeRWC) Close() error {
	_ = p.PipeReader.Close()
	if p.PipeWriter != nil {
		return p.PipeWriter.Close()
	}
	return nil
}

// TestResilientStreamAgainstRealFraming pins the error classification to the
// actual jsonrpc2 stream implementation: a library upgrade that changes the
// decode-error message turns this red instead of silently making malformed
// frames fatal again.
func TestResilientStreamAgainstRealFraming(t *testing.T) {
	reader, writer := io.Pipe()
	stream := resilientStream{inner: jsonrpc2.NewStream(pipeRWC{reader, nil})}

	go func() {
		// 1. framed garbage body (invalid JSON)
		garbage := "{oops"
		fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(garbage), garbage)
		// 2. framed valid JSON that is not a valid message (no method, no id)
		empty := "{}"
		fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(empty), empty)
		// 3. a valid notification
		valid := `{"jsonrpc":"2.0","method":"initialized","params":{}}`
		fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n%s", len(valid), valid)
		_ = writer.Close()
	}()

	msg, _, err := stream.Read(context.Background())
	if err != nil {
		t.Fatalf("expected malformed frames to be skipped, got error: %v", err)
	}
	req, ok := msg.(jsonrpc2.Request)
	if !ok || req.Method() != "initialized" {
		t.Fatalf("expected the valid notification, got %#v", msg)
	}

	// Pipe closed: the next read is a genuine IO error and must be fatal.
	if _, _, err := stream.Read(context.Background()); err == nil {
		t.Fatal("expected fatal error after pipe close")
	}
}
