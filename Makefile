.PHONY: build run test clean check

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
	rm -f $(APP_NAME)
	@echo "==> Removing extracted workspace..."
	rm -rf ~/.picoclaw/workspace/
	@echo "==> Clean complete!"
