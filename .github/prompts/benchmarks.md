# Prompt: Benchmark mở rộng cho unologger

Mục tiêu:
- Cung cấp benchmark matrix để đánh giá throughput/alloc cho các cấu hình: JSON vs text, workers, batch size, non-blocking.

Phạm vi:
- Tạo `unologger_bench_test.go` với nhiều benchmark table-driven.
- In kết quả alloc/op, MB/s nếu phù hợp.

Ràng buộc:
- Không phụ thuộc ngoài.

Acceptance criteria:
- `go test -bench . -benchmem` chạy pass; benchmark có ý nghĩa.