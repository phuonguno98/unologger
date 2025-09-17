/*
Package unologger provides a flexible and feature-rich logging library for Go applications.
It is designed for high-performance, concurrency-safe logging with extensive customization options.

Key Features:
  - Level-based logging (DEBUG, INFO, WARN, ERROR, FATAL).
  - Asynchronous processing with worker pools and non-blocking enqueue.
  - Log batching to optimize I/O operations.
  - Data masking for sensitive information using both regex patterns and JSON field names.
  - Extensible hook system for custom log processing (synchronous/asynchronous, with timeouts and error handling).
  - Log file rotation (using lumberjack) with configurable size, age, backups, and compression.
  - Support for multiple output writers (stdout, stderr, and additional custom writers).
  - Dynamic configuration allows changing logger settings at runtime without application restart.
  - OpenTelemetry integration for propagating trace and span IDs in log contexts.
  - Context-aware logging for attaching module names, flow IDs, and custom attributes.
  - Internal pooling of log entries and batches to reduce garbage collection overhead.

Example Usage:

	package main

	import (
		"context"
		"fmt"
		"os"
		"time"

		"github.com/phuonguno98/unologger"
	)

	func main() {
		// Configure the logger
		cfg := unologger.Config{
			MinLevel:    unologger.INFO,
			Timezone:    "UTC",
			JSON:        true, // Enable JSON output
			Buffer:      1024,
			Workers:     2,
			NonBlocking: true,
			Batch:       unologger.BatchConfig{Size: 5, MaxWait: 200 * time.Millisecond},
			Rotation: unologger.RotationConfig{
				Enable:    true,
				Filename:  "app.log",
				MaxSizeMB: 10,
			},
			Stdout: os.Stdout,
			RegexPatternMap: map[string]string{
				`\b\d{16}\b`: "****MASKED_CARD****",
			},
		}
		unologger.InitLoggerWithConfig(cfg)
		defer func() {
			if err := unologger.Close(5 * time.Second); err != nil {
				fmt.Printf("Error closing logger: %v\n", err)
			}
		}()

		// Create a context with some metadata
		ctx := context.Background()
		ctx = unologger.WithModule(ctx, "payment-service").Context()
		ctx = unologger.WithFlowID(ctx, "req-12345")
		ctx = unologger.WithAttrs(ctx, unologger.Fields{"user_id": "u007", "transaction_id": "tx-abc"})

		// Get a context-aware logger and log messages
		log := unologger.GetLogger(ctx)
		log.Info("Processing payment for amount %f", 123.45)
		log.Debug("Sensitive data: card number 1234567890123456") // This will be masked

		// Dynamically change log level
		unologger.GlobalLogger().SetMinLevel(unologger.DEBUG)
		log.Debug("This debug message is now visible.")

		// Use an adapter for external packages
		adapter := unologger.NewAdapter(log)
		adapter.Error("An error occurred in an external component.")
	}
*/
package unologger