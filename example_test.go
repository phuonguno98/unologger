package unologger_test

import (
	"context"
	"time"

	"github.com/phuonguno98/unologger"
)

// Example demonstrates basic usage of unologger as a Go library.
func Example() {
	// Initialize global logger with minimal setup.
	unologger.InitLogger(unologger.INFO, "UTC")
	defer unologger.Close(2 * time.Second)

	// Create a context with module name and a flow id.
	ctx := context.Background()
	ctx = unologger.WithModule(ctx, "example").Context()
	ctx = unologger.WithFlowID(ctx, "flow-demo-1")

	// Get a context-aware logger and log a message.
	log := unologger.GetLogger(ctx)
	log.Info("hello %s", "library")

	// Lưu ý: Ví dụ này ghi log ra stdout/stderr với timestamp nên không kiểm tra Output cố định.
}
