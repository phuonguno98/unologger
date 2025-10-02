# Prompt: Masking nâng cao (nested path, wildcard, partial)

Mục tiêu:
- Hỗ trợ mask theo đường dẫn field (vd: `user.password`), wildcard một cấp (`user.*.password`), và mask một phần (giữ N ký tự).

Phạm vi:
- Mở rộng `MaskFieldRule` với các trường: `Paths []string`, `KeepLast int` (hoặc `Partial struct` linh hoạt).
- Nâng cấp traversal JSON để mask theo path với wildcard, giữ tương thích `Keys` cũ.

Ràng buộc:
- Backward-compatible; không ảnh hưởng performance khi không dùng tính năng mới.

Acceptance criteria:
- Unit tests: mask nested, wildcard, partial; fallback khi JSON parse lỗi vẫn áp dụng regex rules.

Kiểm thử tối thiểu:
- Log JSON nhiều cấp; xác nhận các trường theo path được mask đúng, phần còn lại không bị ảnh hưởng.