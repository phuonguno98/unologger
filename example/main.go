// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/phuonguno98/unologger"
)

// hookPrint is a sample hook function that demonstrates how to process a HookEvent.
// It prints the event's details (JSON mode, level, module, message) to standard output.
// Hooks are executed as part of the logging pipeline and can be used for custom processing
// like sending logs to external services, filtering, or enriching data.
func hookPrint(ev unologger.HookEvent) error {
	fmt.Printf("[HOOK] json=%v %s %s: %s\n", ev.JSONMode, ev.Level, ev.Module, ev.Message)
	return nil
}

// hookSlow is a sample hook function that simulates a time-consuming operation.
// It's used to demonstrate the hook timeout mechanism configured in HookConfig.
// If a hook takes longer than its configured timeout, it will be considered failed,
// and the logging pipeline will continue without waiting indefinitely.
func hookSlow(_ unologger.HookEvent) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

// main function serves as a comprehensive example demonstrating various features and
// usage patterns of the unologger library. It covers initial configuration, context-aware
// logging, data masking, dynamic configuration changes, OpenTelemetry integration,
// and statistics retrieval.
func main() {
	// 1) Initial logger configuration with a wide range of common features.
	// This Config struct allows fine-grained control over the logger's behavior,
	// including log levels, output formats, concurrency, batching, rotation,
	// data masking rules, and integration points like hooks and OpenTelemetry.
	cfg := unologger.Config{
		MinLevel:    unologger.DEBUG, // Set the minimum log level to DEBUG to capture all messages.
		Timezone:    "Asia/Ho_Chi_Minh", // Specify the timezone for log timestamps.
		JSON:        false, // Initially set to false for plain text output.
		Buffer:      1024,  // Size of the internal channel buffer for log entries.
		Workers:     2,     // Number of goroutines processing log entries concurrently.
		NonBlocking: true,  // Enable non-blocking enqueue operations to prevent application slowdown.
		DropOldest:  true,  // If non-blocking and the buffer is full, drop the oldest log entry.
		Batch:       unologger.BatchConfig{Size: 5, MaxWait: 400 * time.Millisecond}, // Configure batching to reduce I/O.
		Retry:       unologger.RetryPolicy{MaxRetries: 2, Backoff: 80 * time.Millisecond, Exponential: true, Jitter: 20 * time.Millisecond}, // Define retry strategy for failed writes.
		Rotation: unologger.RotationConfig{
			Enable:     true,            // Enable rotation.
			Filename:   "example/app.log", // Base filename for rotated logs.
			MaxSizeMB:  5,               // Rotate when file size exceeds 5 MB.
			MaxBackups: 2,               // Keep up to 2 old log files.
			MaxAge:     7,               // Delete old log files after 7 days.
			Compress:   true,            // Compress old log files.
		},
		Stdout: os.Stdout, // Direct standard output logs to os.Stdout.
		Stderr: os.Stderr, // Direct error output logs to os.Stderr.
		RegexPatternMap: map[string]string{
			`\b\d{16}\b`:                                       "****MASKED_CARD****",          // Mask 16-digit numbers (e.g., credit card numbers).
			`(?i)authorization:\s*Bearer\s+\S+`:                "authorization: Bearer ****MASKED****", // Mask Bearer tokens in Authorization headers.
			`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`: "***@masked.email",           // Mask email addresses.
		},
		JSONFieldRules: []unologger.MaskFieldRule{
			{Keys: []string{"password", "token", "secret"}, Replacement: "****"},
		},
		Hooks:      []unologger.HookFunc{hookPrint, hookSlow},
		Hook:       unologger.HookConfig{Async: true, Workers: 2, Queue: 1024, Timeout: 200 * time.Millisecond},
		EnableOTEL: false, // OpenTelemetry integration is initially disabled.
	}
	unologger.InitLoggerWithConfig(cfg) // Initialize the global logger with the defined configuration.
	// Ensure the logger is gracefully closed when the main function exits.
	// This flushes any buffered logs and stops worker goroutines.
	defer func() {
		if err := unologger.Close(2 * time.Second); err != nil {
			fmt.Println("Close timeout:", err)
		}
	}()

	// 2) Setting up initial context and using LoggerWithCtx for context-aware logging.
	// Context allows attaching metadata (like module, trace ID, user ID) to log entries,
	// which is crucial for tracing and debugging in distributed systems.
	ctx := context.Background()
	ctx = unologger.WithModule(ctx, "main-service").Context() // Attach a module name to the context.
	ctx = unologger.WithFlowID(ctx, "flow-001")               // Attach a custom flow ID.
	ctx = unologger.WithAttrs(ctx, unologger.Fields{"user_id": "u001"}) // Attach custom key-value attributes.
	ctx = unologger.EnsureTraceIDCtx(ctx)                     // Ensure a trace ID is present in the context (generates one if missing).
	log := unologger.GetLogger(ctx)                           // Retrieve a logger instance bound to the current context.

	// 3) Demonstrating logging at various levels and text masking in action.
	// The configured regex and field masking rules will automatically apply to these messages.
	log.Debug("Starting payment processing for user %s", "u001")
	log.Info("Payment successful for order %d", 1001)
	log.Warn("Low account balance for user %s", "u001")
	log.Error("Error connecting to bank")
	log.Info("Testing card masking: 1234567812345678 and email: user@example.com")
	log.Info("Header authorization: Bearer very-secret-token-abcxyz")

	// 4) Demonstrating the Adapter for integrating unologger with external packages
	// that might use a simpler logging interface. The adapter allows these packages
	// to log through unologger's full pipeline.
	adapter := unologger.NewAdapter(log)
	doExternal(adapter)

	// 5) Demonstrating dynamic configuration changes at runtime.
	// The logger's behavior can be altered without restarting the application.
	unologger.GlobalLogger().SetMinLevel(unologger.WARN) // Dynamically change the minimum log level to WARN.
	log.Debug("This log will be ignored (DEBUG < WARN) due to dynamic min-level change.")
	log.Warn("This log will be written (>= WARN) after dynamic min-level change.")
	unologger.GlobalLogger().SetBatchConfig(unologger.BatchConfig{Size: 3, MaxWait: 200 * time.Millisecond}) // Dynamically update batching configuration.

	// 6) Adding an extra writer and dynamically changing logger outputs.
	// This shows how logs can be directed to multiple destinations simultaneously.
	tmpFile, _ := os.CreateTemp("", "extra-log-*.log") // Create a temporary file for an extra writer.
	unologger.GlobalLogger().AddExtraWriter("tempfile", tmpFile) // Add the temporary file as an additional log destination.
	memBuf := &bytes.Buffer{} // Create an in-memory buffer to capture logs.
	unologger.GlobalLogger().SetOutputs(nil, nil, []io.Writer{memBuf}, []string{"mem-buf"}) // Redirect logs to memBuf (and other existing writers).
	log.Info("Logging in parallel: stdout/stderr, rotation, tempfile, and mem-buf.")

	// 7) Updating hooks at runtime.
	// Existing hooks can be replaced or modified dynamically.
	unologger.GlobalLogger().SetHooks([]unologger.HookFunc{
		func(ev unologger.HookEvent) error {
			fmt.Printf("[HOOK2] json=%v %s %s: %s\n", ev.JSONMode, ev.Level, ev.Module, ev.Message)
			return fmt.Errorf("demo hook error") // Simulate a hook error.
		},
	})

	// 8) Enabling JSON mode and demonstrating field-level JSON masking.
	// The logger can switch between text and JSON output dynamically.
	unologger.GlobalLogger().SetJSONFormat(true) // Switch logger output to JSON format.
	// Log a JSON string with sensitive fields that will be masked by JSONFieldRules.
	log.Info(`{"event":"login","user":"u001","password":"123456","token":"abcdef"}`)

	// 9) Changing timezone and enabling OpenTelemetry integration dynamically.
	if err := unologger.GlobalLogger().SetTimezone("UTC"); err != nil {
		fmt.Println("Failed to change timezone:", err)
	}
	unologger.GlobalLogger().SetEnableOTEL(true) // Enable OpenTelemetry integration.

	// 10) Logging with OpenTelemetry trace/span context.
	// Logs will automatically include trace and span IDs when OTEL is enabled and context is propagated.
	ctxOT, span := otel.Tracer("demo-tracer").Start(ctx, "demo-operation") // Start a new OTel span.
	unologger.GetLogger(ctxOT).Info("Log with trace/span from OTel.")      // Log using the context with OTel span.
	span.End()                                                             // End the OTel span.

	// 11) Reinitializing the global logger with a completely new configuration.
	// This demonstrates a full reset and re-application of logger settings.
	cfg2 := cfg // Start with a copy of the initial configuration.
	cfg2.JSON = true
	cfg2.Workers = 1
	cfg2.MinLevel = unologger.INFO
	if _, err := unologger.ReinitGlobalLogger(cfg2, 2*time.Second); err != nil {
		fmt.Println("Failed to reinitialize logger:", err)
	}
	unologger.GetLogger(ctx).Info("Log after ReinitGlobalLogger (JSON mode).")

	// 12) Removing an extra writer and closing the temporary file.
	unologger.GlobalLogger().RemoveExtraWriter("tempfile") // Remove the previously added temporary file writer.
	if tmpFile != nil {
		_ = tmpFile.Close() // Close the temporary file.
	}

	// 13) Printing logger statistics.
	// unologger provides various counters for monitoring its internal operations.
	dropped, written, batches, werrs, herrs, qlen, wstats, hookErrLog := unologger.Stats()
	fmt.Printf("Stats: dropped=%d written=%d batches=%d writeErrs=%d hookErrs=%d queue=%d writers=%v hookErrLog=%d\n",
		dropped, written, batches, werrs, herrs, qlen, wstats, len(hookErrLog))

	// 14) Logging a few more entries to observe batch flush behavior.
	// This ensures that any remaining buffered logs are processed.
	for i := 0; i < 5; i++ {
		unologger.GetLogger(ctx).Info("Batch log %d", i+1)
	}
	time.Sleep(600 * time.Millisecond) // Give some time for the batch to flush.

	// 15) (Optional) Calling Fatal to demonstrate program exit behavior.
	// Fatal logs a message and then terminates the application.
	if os.Getenv("DEMO_FATAL") == "1" {
		unologger.GetLogger(ctx).Fatal("Demo FATAL: application will exit", nil, unologger.Fields{"exit_code": 1})
	}
}

// doExternal is a helper function simulating an external package or module
// that uses a simplified logging interface (unologger.Adapter) to log messages.
// This demonstrates how unologger can be integrated into existing codebases
// without requiring them to directly use unologger's full API.
func doExternal(adapter *unologger.Adapter) {
	adapter.Info("Called from external package (SimpleLogger).")
	adapter.Warn("Warning from external package.")
	adapter.Error("Error from external package.")
	// Note: Calling Fatal from an external package might be undesirable in some architectures.
	// This example avoids it unless explicitly enabled via an environment variable.
}
