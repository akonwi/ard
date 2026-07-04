package lsp

import (
	"context"
	"encoding/json"
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
