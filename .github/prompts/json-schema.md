# Prompt: Chuẩn hóa JSON schema đầu ra

Mục tiêu:
- Bảo đảm schema thống nhất: `time, level, module, message, trace_id, span_id, flow_id, attrs, fields`.

Phạm vi:
- Cập nhật `JSONFormatter` để luôn include `span_id` (nếu có) là field top-level (không chỉ trong attrs).
- Cập nhật GoDoc/README mô tả schema; ví dụ minh họa.

Ràng buộc:
- Backward-compatible: không thay đổi ý nghĩa các field cũ, chỉ bổ sung khi có dữ liệu.

Acceptance criteria:
- Unit tests: khi có span, đầu ra chứa `span_id` ở top-level.