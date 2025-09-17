// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/phuonguno98/unologger"
	"go.opentelemetry.io/otel"
)

// hookPrint is a simple hook that prints selected event details to the console.
func hookPrint(ev unologger.HookEvent) error {
	fmt.Printf("[HOOK] Level=%s Module=%s: %s\n", ev.Level, ev.Module, ev.Message)
	return nil
}

// hookSlow simulates a slow hook to demonstrate the timeout feature.
func hookSlow(_ unologger.HookEvent) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

// main provides a comprehensive demonstration of the unologger library's features.
func main() {
	// 1. Initial Configuration
	// Configure the logger with a variety of common features.
	cfg := unologger.Config{
		MinLevel:    unologger.DEBUG,
		Timezone:    "Asia/Ho_Chi_Minh",
		JSON:        false, // Start with plain text logs.
		Buffer:      1024,
		Workers:     2,
		NonBlocking: true,
		DropOldest:  true,
		Batch:       unologger.BatchConfig{Size: 5, MaxWait: 400 * time.Millisecond},
		Retry:       unologger.RetryPolicy{MaxRetries: 2, Backoff: 80 * time.Millisecond, Exponential: true},
		Rotation: unologger.RotationConfig{
			Enable:    true,
			Filename:  "example/app.log",
			MaxSizeMB: 5,
			Compress:  true,
		},
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		RegexPatternMap: map[string]string{
			`\b\d{16}\b`:                        `[REDACTED_CARD]`,
			`(?i)authorization:\s*Bearer\s+\S+`: `authorization: Bearer [REDACTED]`,
		},
		JSONFieldRules: []unologger.MaskFieldRule{
			{Keys: []string{"password", "token"}, Replacement: "[REDACTED]"},
		},
		Hooks:      []unologger.HookFunc{hookPrint, hookSlow},
		Hook:       unologger.HookConfig{Async: true, Workers: 2, Timeout: 200 * time.Millisecond},
		EnableOTel: true,
	}
	unologger.InitLoggerWithConfig(cfg)

	// Defer Close to ensure all logs are flushed before the application exits.
	defer func() {
		fmt.Println("\nClosing logger...")
		if err := unologger.Close(2 * time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "Logger close timeout: %v\n", err)
		}
	}()

	// 2. Context-Aware Logging
	// Create a logger with contextual metadata that will be included in all its log entries.
	ctx := context.Background()
	ctx = unologger.WithModule(ctx, "payment-service").Context()
	ctx = unologger.WithFlowID(ctx, "flow-abc-123")
	ctx = unologger.EnsureTraceIDCtx(ctx) // Ensure a trace ID exists.
	log := unologger.GetLogger(ctx).WithAttrs(unologger.Fields{"user_id": "u001"})

	fmt.Println("---" + " Logging with initial text format ---")
	log.Info("Processing payment for order %d", 1001)
	log.Warn("Credit card 1234567812345678 will be masked.")
	log.Info("Header 'authorization: Bearer very-secret-token' will be masked.")

	// 3. Using the Adapter
	// The adapter provides a simplified interface for external packages.
	fmt.Println("\n---" + " Logging via Adapter ---")
	doExternal(unologger.NewAdapter(log))

	// 4. Dynamic Configuration
	// Change the logger's behavior at runtime without a restart.
	fmt.Println("\n---" + " Demonstrating dynamic configuration ---")
	unologger.GlobalLogger().SetMinLevel(unologger.WARN)
	log.Info("This INFO log will be ignored now (INFO < WARN).") // This log will be dropped due to MinLevel being WARN.
	log.Warn("This WARN log is visible after changing level.")

	// 5. Dynamic JSON Mode & Field Masking
	fmt.Println("\n---" + " Switching to JSON format ---")
	unologger.GlobalLogger().SetJSONFormat(true)
	log.Error("This error will be in JSON format.")
	// This INFO log will be dropped because MinLevel is still WARN.
	log.Info(`{"event":"login", "user":"demo", "password":"a-very-secret-password"}`)

	// 6. OpenTelemetry Integration
	// Automatically include trace and span IDs in logs.
	fmt.Println("\n---" + " Logging with OpenTelemetry Span ---")
	ctxOT, span := otel.Tracer("demo-tracer").Start(ctx, "demo-op")
	// This INFO log will be dropped because MinLevel is still WARN.
	unologger.GetLogger(ctxOT).Info("This log includes OTel trace and span IDs.")
	span.End()

	// 7. Re-initializing the Global Logger
	// Replace the global logger with a completely new configuration.
	fmt.Println("\n---" + " Re-initializing global logger ---")
	cfg2 := cfg
	cfg2.JSON = false // Switch back to text format.
	cfg2.MinLevel = unologger.INFO // Set MinLevel back to INFO.
	if _, err := unologger.ReinitGlobalLogger(cfg2, 2*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reinitialize logger: %v\n", err)
	}
	// IMPORTANT: After ReinitGlobalLogger, the 'log' variable (LoggerWithCtx)
	// still points to the OLD logger instance. To log with the NEW configuration,
	// you must re-create the LoggerWithCtx instance.
	newLog := unologger.GetLogger(ctx).WithAttrs(unologger.Fields{"user_id": "u001"})
	newLog.Info("This log is in text format again after re-initialization.")

	// 8. Final Stats
	// Retrieve and print runtime statistics.
	time.Sleep(500 * time.Millisecond) // Allow time for last logs to be processed.
	fmt.Println("\n---" + " Final Logger Statistics ---")
	dropped, written, batches, wErrs, hErrs, qLen, wStats, hLog := unologger.Stats()
	fmt.Printf("Queue Length: %d\n", qLen)
	fmt.Printf("Processed: written=%d, batches=%d\n", written, batches)
	fmt.Printf("Errors: dropped=%d, write_errors=%d, hook_errors=%d\n", dropped, wErrs, hErrs)
	fmt.Printf("Writer Errors Detail: %v\n", wStats)
	fmt.Printf("Hook Errors Detail: %d entries\n", len(hLog))

	// 9. Fatal Log
	// Demonstrate a fatal log call which terminates the application.
	if os.Getenv("DEMO_FATAL") == "1" {
		fmt.Println("\n---" + " Demonstrating Fatal Log (will exit) ---")
		// Add fields via WithAttrs before calling Fatal.
		fatalLogger := log.WithAttrs(unologger.Fields{"exit_code": 1})
		fatalLogger.Fatal("This is a fatal error, application will now exit.")
	}
}

// doExternal simulates a call to an external package that uses the simplified Adapter.
func doExternal(log unologger.SimpleLogger) {
	log.Info("Log from an external component.")
	log.Warn("Warning from an external component.")
}
