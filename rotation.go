// Copyright (c) 2025 Nguyễn Thanh Phương
// This source code is licensed under the MIT License found in the LICENSE file.

// Package unologger - rotation.go
// Cung cấp cơ chế khởi tạo writer xoay vòng (rotation) cho file log.
// Rotation giúp tránh việc file log quá lớn hoặc quá cũ, đồng thời hỗ trợ lưu trữ và quản lý log hiệu quả.
// Sử dụng thư viện lumberjack để xoay file theo dung lượng, thời gian và số lượng file backup.

package unologger

import (
	"io"

	"gopkg.in/natefinch/lumberjack.v2"
)

// initRotationWriter khởi tạo writer xoay file nội bộ nếu RotationConfig.Enable=true.
// Nếu không bật rotation hoặc thiếu Filename, trả về nil.
// Sử dụng lumberjack.Logger để xoay file theo dung lượng/thời gian/backups.
//
// Tham số:
//   - cfg: RotationConfig chứa thông tin cấu hình xoay file.
//
// Trả về:
//   - io.Writer: writer đã cấu hình rotation, hoặc nil nếu không bật rotation.
func initRotationWriter(cfg RotationConfig) io.Writer {
	if !cfg.Enable || cfg.Filename == "" {
		return nil
	}
	return &lumberjack.Logger{
		Filename:   cfg.Filename,   // Đường dẫn file log
		MaxSize:    cfg.MaxSizeMB,  // Dung lượng tối đa (MB) trước khi xoay
		MaxAge:     cfg.MaxAge,     // Số ngày lưu file log cũ
		MaxBackups: cfg.MaxBackups, // Số file log cũ tối đa
		Compress:   cfg.Compress,   // Nén file log cũ
	}
}
