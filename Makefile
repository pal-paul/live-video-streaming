.PHONY: build run test clean install deps

# Variables
BINARY_NAME=video-service
MAIN_PATH=cmd/server/main.go
BUILD_DIR=bin

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	@go run $(MAIN_PATH)

# Run with environment variables
run-dev:
	@echo "Running in development mode..."
	@export GIN_MODE=debug && go run $(MAIN_PATH)

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go get -u github.com/gin-gonic/gin
	@go get -u github.com/gin-contrib/cors
	@go get -u github.com/google/uuid
	@go mod tidy
	@echo "Dependencies installed"

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@go clean
	@echo "Clean complete"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	@golangci-lint run ./...

# Create directories
setup:
	@echo "Setting up project structure..."
	@mkdir -p cmd/server
	@mkdir -p internal/handlers
	@mkdir -p pkg/storage
	@mkdir -p pkg/broadcast
	@mkdir -p templates
	@mkdir -p static
	@echo "Setup complete"

# Run with hot reload (requires air)
dev:
	@echo "Starting development server with hot reload..."
	@air

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	@GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PATH)
	@GOOS=darwin GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PATH)
	@GOOS=darwin GOARCH=arm64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PATH)
	@GOOS=windows GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "Cross-platform build complete"

# Help
help:
	@echo "Available commands:"
	@echo "  make build      - Build the application"
	@echo "  make run        - Run the application"
	@echo "  make run-dev    - Run in development mode"
	@echo "  make deps       - Install dependencies"
	@echo "  make test       - Run tests"
	@echo "  make clean      - Clean build artifacts"
	@echo "  make fmt        - Format code"
	@echo "  make lint       - Lint code"
	@echo "  make setup      - Create project directories"
	@echo "  make build-all  - Build for all platforms"
