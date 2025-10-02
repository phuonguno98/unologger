# Prompt: Thiết lập CI (build, vet, fmt-check, test, coverage) cho unologger

Mục tiêu:
- Tạo workflow GitHub Actions `.github/workflows/ci.yml` chạy trên push/pull_request để build/vet/fmt-check/test (race)/coverage với matrix OS và Go.

Phạm vi:
- jobs.build-and-test:
  - matrix: os = [ubuntu-latest, windows-latest], go = [1.25.x].
  - Steps:
    - actions/checkout@v4
    - actions/setup-go@v5 (go-version: matrix)
    - Cache module/build: actions/cache@v4 (khóa theo OS + Go + go.sum)
    - Run:
      - go mod download
      - go vet ./...
      - Check format:
        - Linux: `diff -u <(echo -n) <(gofmt -s -l .)` và fail nếu có output
        - Windows (pwsh): `$fmt = gofmt -s -l .; if ($fmt) { echo $fmt; exit 1 }`
      - go build ./...
      - go test -race -coverprofile=coverage.out ./...
    - Upload artifact coverage.out qua actions/upload-artifact@v4

Ràng buộc:
- Không cài thêm linter ngoài nếu chưa cần; có thể thêm job golangci-lint sau.
- Quyền tối thiểu: `permissions: contents: read`.

Acceptance criteria:
- PR mở ra sẽ kích hoạt workflow, hiển thị trạng thái pass/fail, có artifact coverage.