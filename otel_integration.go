// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - otel_integration.go
// Cung cấp các hàm tích hợp với OpenTelemetry (OTel) để tự động lấy trace_id, span_id từ context
// và gắn vào log entry. Điều này giúp liên kết log với dữ liệu trace phục vụ việc quan sát hệ thống.

package unologger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// extractOTELTraceID cố gắng lấy trace_id từ OpenTelemetry context.
// Trả về chuỗi rỗng nếu không tìm thấy hoặc không có OTEL.
func extractOTELTraceID(ctx context.Context) string {
	if span := trace.SpanFromContext(ctx); span != nil {
		sc := span.SpanContext()
		if sc.HasTraceID() {
			return sc.TraceID().String()
		}
	}
	return ""
}

// extractOTELSpanID cố gắng lấy span_id từ OpenTelemetry context.
// Trả về chuỗi rỗng nếu không tìm thấy hoặc không có OTEL.
func extractOTELSpanID(ctx context.Context) string {
	if span := trace.SpanFromContext(ctx); span != nil {
		sc := span.SpanContext()
		if sc.HasSpanID() {
			return sc.SpanID().String()
		}
	}
	return ""
}

// formatTraceSpanID trả về chuỗi kết hợp trace_id và span_id (nếu có).
// Ví dụ: "traceid/spanid" hoặc chỉ "traceid" nếu không có span_id.
func formatTraceSpanID(ctx context.Context) string {
	tid := extractOTELTraceID(ctx)
	sid := extractOTELSpanID(ctx)
	if tid != "" && sid != "" {
		return tid + "/" + sid
	}
	return tid
}

// AttachOTELTrace gắn trace_id và span_id từ OTel vào context log.
// Nếu không có trace hợp lệ, trả về nguyên context ban đầu.
func AttachOTELTrace(ctx context.Context) context.Context {
	tid := extractOTELTraceID(ctx)
	sid := extractOTELSpanID(ctx)
	if tid == "" {
		return ctx
	}
	ctx = WithTraceID(ctx, tid)
	if sid != "" {
		// Lưu span_id như một attr để hook hoặc writer có thể sử dụng
		ctx = WithAttrs(ctx, map[string]string{"span_id": sid})
	}
	return ctx
}
