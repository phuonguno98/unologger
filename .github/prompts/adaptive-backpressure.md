# Prompt: Backpressure thích ứng cho unologger

Mục tiêu:
- Khi quá tải, tự động chuyển chế độ (drop-oldest hoặc tăng MinLevel tạm thời) để bảo vệ hệ thống.

Phạm vi:
- Thêm `AdaptiveBackpressure` trong `Config` (threshold %, hysteresis, cool-down).
- Theo dõi tải hàng đợi; khi vượt ngưỡng, bật chế độ; khi hồi phục, revert.

Ràng buộc:
- Disabled by default.

Acceptance criteria:
- Unit tests: mô phỏng overload (writer block) → chế độ kích hoạt; khi tải giảm → revert.