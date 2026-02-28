.PHONY: build run test clean check release build-all

# The name of our binary
APP_NAME := son-of-anthon

# The directory containing the main package
CMD_DIR := ./cmd/son-of-anthon

# Default target
all: build

build: check
	@echo "==> Building $(APP_NAME)..."
	go build -o $(APP_NAME) $(CMD_DIR)
	@echo "==> Build complete!"

build-all: build-linux-amd64 build-linux-arm64 build-windows
	@echo "==> All platforms built!"

build-linux-amd64:
	@echo "==> Building Linux AMD64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(APP_NAME)-linux-amd64 $(CMD_DIR)

build-linux-arm64:
	@echo "==> Building Linux ARM64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(APP_NAME)-linux-arm64 $(CMD_DIR)

build-windows:
	@echo "==> Building Windows AMD64..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o $(APP_NAME)-windows-amd64.exe $(CMD_DIR)

run: build
	@echo "==> Running $(APP_NAME) gateway..."
	./$(APP_NAME) gateway

test: check
	@echo "==> Running tests with race detector..."
	go test -v -race ./...

check:
	@echo "==> Running Go vet and formatting checks..."
	go fmt ./...
	go vet ./...

clean:
	@echo "==> Cleaning up..."
	go clean
	rm -f $(APP_NAME)-*
	rm -f $(APP_NAME)
	@echo "==> Removing extracted workspace..."
	rm -rf ~/.picoclaw/workspace/
	@echo "==> Clean complete!"

release: build-all
	@echo "==> Creating release..."
	@echo "Artifacts ready for upload:"
	@ls -lh $(APP_NAME)-* 2>/dev/null || echo "No cross-platform builds found"
	@echo ""
	@echo "To create GitHub release, push to main branch or run:"
	@echo "  git tag v0.0.x && git push origin main --tags"
