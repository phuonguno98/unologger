# Prompt: Xuất metrics bằng OpenTelemetry cho unologger

Mục tiêu:
- Expose metrics counters/gauges về dropped, written, batches, writeErrs, hookErrs, queueLen và per-writer errors.

Phạm vi:
- Thêm tùy chọn `EnableMetrics bool` vào `Config`.
- Dùng `go.opentelemetry.io/otel/metric` để tạo instruments (không cứng phụ thuộc Prometheus).
- Counters: written, dropped, batches, writeErrs, hookErrs; Gauge: queueLen; Counter có label `writer` cho writer errors.
- Wiring cập nhật giá trị tại các điểm biến đổi trong pipeline/hook/write/close.

Ràng buộc:
- No-op nếu `EnableMetrics=false`.
- GoDoc, header 2025, không gây overhead đáng kể khi tắt.

Acceptance criteria:
- Unit tests với test SDK của OTel (hoặc exporter giả) xác nhận số đo tăng đúng khi phát sinh sự kiện.

Kiểm thử tối thiểu:
- Sinh 1 batch log -> written tăng; chặn writer -> writeErr tăng; simulate hook timeout -> hookErr tăng; queueLen phản ánh.