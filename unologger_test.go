// Copyright 2025 Nguyen Thanh Phuong. All rights reserved.

package unologger

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// blockingWriter is a test helper writer that blocks writes until unblocked.
type blockingWriter struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	blockC chan struct{}
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{blockC: make(chan struct{})}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.blockC // Block until explicitly unblocked in test.
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *blockingWriter) unblock() {
	select {
	case <-w.blockC:
		// Already unblocked.
	default:
		close(w.blockC)
	}
}

func (w *blockingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

func TestRoutingAndLevelFilter(t *testing.T) {
	out := &bytes.Buffer{}
	errb := &bytes.Buffer{}
	cfg := Config{
		MinLevel: INFO,
		Timezone: "UTC",
		JSON:     false,
		Buffer:   64,
		Workers:  1,
		Batch:    BatchConfig{Size: 1, MaxWait: 50 * time.Millisecond},
		Stdout:   out,
		Stderr:   errb,
	}
	l := NewDetachedLogger(cfg)
	defer func() { _ = CloseDetached(l, 2*time.Second) }()

	lw := l.WithContext(context.Background())
	lw.Info("hello")
	lw.Warn("warn here") // WARN should go to stderr

	// Ensure flush.
	require.NoError(t, CloseDetached(l, 2*time.Second))
	// No further writes should occur; re-create for reading.

	// Validate routing.
	require.Contains(t, out.String(), "hello")
	require.NotContains(t, out.String(), "warn here")
	require.Contains(t, errb.String(), "warn here")
}

func TestLevelFiltering(t *testing.T) {
	out := &bytes.Buffer{}
	cfg := Config{
		MinLevel: WARN,
		Timezone: "UTC",
		Buffer:   64,
		Workers:  1,
		Batch:    BatchConfig{Size: 1, MaxWait: 50 * time.Millisecond},
		Stdout:   out,
		Stderr:   out, // Route WARN and above to the same buffer for assertion.
	}
	l := NewDetachedLogger(cfg)
	defer func() { _ = CloseDetached(l, 2*time.Second) }()

	lw := l.WithContext(context.Background())
	lw.Info("this should be filtered")
	lw.Warn("this should appear")

	require.NoError(t, CloseDetached(l, 2*time.Second))
	s := out.String()
	require.NotContains(t, s, "this should be filtered")
	require.Contains(t, s, "this should appear")
}

func TestJSONMasking(t *testing.T) {
	out := &bytes.Buffer{}
	cfg := Config{
		MinLevel:       INFO,
		Timezone:       "UTC",
		JSON:           true,
		Buffer:         64,
		Workers:        1,
		Batch:          BatchConfig{Size: 1, MaxWait: 50 * time.Millisecond},
		Stdout:         out,
		JSONFieldRules: []MaskFieldRule{{Keys: []string{"password", "token"}, Replacement: "[REDACTED]"}},
	}
	l := NewDetachedLogger(cfg)
	defer func() { _ = CloseDetached(l, 2*time.Second) }()

	lw := l.WithContext(context.Background())
	// This will be treated as a string message, but masking should rewrite the JSON string before formatting.
	lw.Info(`{"event":"login","user":"u","password":"secret"}`)

	require.NoError(t, CloseDetached(l, 2*time.Second))
	// Parse the top-level JSON line and examine the message field to avoid string escaping issues.
	type top struct {
		Time    string `json:"time"`
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	var line top
	require.NoError(t, json.Unmarshal(out.Bytes(), &line))
	require.NotEmpty(t, line.Message)

	// The message itself is a JSON string. Parse it and verify masking.
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(line.Message), &payload))
	require.Equal(t, "[REDACTED]", payload["password"])
}

func TestNonBlockingDropsWhenQueueFull(t *testing.T) {
	bw := newBlockingWriter()
	cfg := Config{
		MinLevel:    DEBUG,
		Timezone:    "UTC",
		Buffer:      2,
		Workers:     1,
		NonBlocking: true,
		DropOldest:  false,
		Batch:       BatchConfig{Size: 1, MaxWait: time.Second},
		Stdout:      bw,
		Stderr:      bw,
	}
	l := NewDetachedLogger(cfg)
	defer func() { _ = CloseDetached(l, 2*time.Second) }()

	lw := l.WithContext(context.Background())
	// Issue many logs while writer is blocked to fill channel and cause drops.
	for i := 0; i < 50; i++ {
		lw.Info("blocked %d", i)
	}

	// Unblock writer and close.
	bw.unblock()
	_ = CloseDetached(l, 2*time.Second)

	// Check stats: some entries should have been dropped.
	dropped, _, _, _, _, _, _, _ := StatsDetached(l)
	require.Greater(t, dropped, int64(0))
}

func TestHookTimeoutErrorRecorded(t *testing.T) {
	out := &bytes.Buffer{}
	slowHook := func(_ HookEvent) error {
		time.Sleep(120 * time.Millisecond)
		return nil
	}
	cfg := Config{
		MinLevel: INFO,
		Timezone: "UTC",
		Buffer:   16,
		Workers:  1,
		Batch:    BatchConfig{Size: 1, MaxWait: 10 * time.Millisecond},
		Stdout:   out,
		Hooks:    []HookFunc{slowHook},
		Hook:     HookConfig{Async: true, Workers: 1, Timeout: 50 * time.Millisecond, Queue: 8},
	}
	l := NewDetachedLogger(cfg)
	defer func() { _ = CloseDetached(l, 2*time.Second) }()

	l.WithContext(context.Background()).Info("trigger hook")
	// Give some time for hook to process and hit timeout.
	time.Sleep(200 * time.Millisecond)
	_ = CloseDetached(l, 2*time.Second)

	_, _, _, _, hookErrs, _, _, hlog := StatsDetached(l)
	require.GreaterOrEqual(t, hookErrs, int64(1))
	require.NotEmpty(t, hlog)
}

func TestSetRotationCreatesSink(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "app.log")
	cfg := Config{MinLevel: INFO, Timezone: "UTC", Stdout: io.Discard, Stderr: io.Discard, Buffer: 16, Workers: 1}
	l := NewDetachedLogger(cfg)
	defer func() { _ = CloseDetached(l, 2*time.Second) }()

	l.SetRotation(RotationConfig{Enable: true, Filename: file, MaxSizeMB: 1, Compress: true})
	require.NotNil(t, l.rotationSink)
}

func BenchmarkLogThroughput_NoOp(b *testing.B) {
	cfg := Config{
		MinLevel: INFO,
		Timezone: "UTC",
		Buffer:   4096,
		Workers:  2,
		Batch:    BatchConfig{Size: 8, MaxWait: 100 * time.Millisecond},
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	}
	l := NewDetachedLogger(cfg)
	defer func() { _ = CloseDetached(l, 2*time.Second) }()
	lw := l.WithContext(context.Background())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lw.Info("hello %d", i)
	}
}

// Ensure example main does not run during tests when imported by tooling.
func TestMain(m *testing.M) {
	code := m.Run()
	os.Exit(code)
}
