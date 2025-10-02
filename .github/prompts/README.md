# Bộ prompt cải tiến cho unologger

Các tệp trong thư mục này là prompt sẵn dùng để tạo issue/PR hoặc chạy với công cụ AI tự động hoá. Mỗi prompt mô tả: mục tiêu, phạm vi, ràng buộc coding, tiêu chí chấp nhận, và kế hoạch kiểm thử tối thiểu.

Lưu ý chung cho tất cả prompt (áp dụng cho code tạo mới/sửa đổi):
- Code Go và GoDoc phải viết bằng tiếng Anh, đúng quy ước trong `doc.go`, dòng đầu là câu tóm tắt, wrap 120 ký tự.
- Header bản quyền cho file code mới: `Copyright 2025 Nguyen Thanh Phuong. All rights reserved.`
- Go >= 1.24 (repo đang dùng 1.25), quản lý module bằng Go Modules.
- Viết tests với `testing` + `testify`, ưu tiên thêm benchmark nếu thay đổi hiệu năng.
- Không phá vỡ API công khai hiện có trừ khi prompt ghi rõ.

## Danh sách prompt

- slog-handler.md — Thêm adapter `log/slog` Handler
- metrics-otel.md — Xuất metrics bằng OpenTelemetry
- masking-advanced.md — Masking nâng cao (nested path, wildcard, partial)
- structured-api.md — API ghi log dạng structured
- json-schema.md — Chuẩn hóa JSON schema (thêm span_id…)
- buffered-writers.md — Tuỳ chọn buffer writers + flush
- health-api.md — API Health snapshot
- adaptive-backpressure.md — Backpressure thích ứng
- benchmarks.md — Benchmark mở rộng
- docs-readme.md — Cập nhật README/docs
- ci-workflow.md — Thiết lập CI build/vet/test/coverage