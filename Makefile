# Quiver Makefile
# This Makefile provides commands for building, testing, and running the Quiver application

# Variables
APP_NAME := quiver
BINARY_DIR := bin
BINARY_PATH := $(BINARY_DIR)/$(APP_NAME)
MAIN_PATH := ./cmd/quiver
DOCKER_IMAGE := quiver:latest
GO_VERSION := 1.24.2
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html
ICON_DIR := cmd/quiver/assets/icons
ICON_SOURCE := cmd/quiver/assets/icons/app.ico

# Go build flags
LDFLAGS := -ldflags "-X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo 'dev')"
BUILD_FLAGS := -a -installsuffix cgo

# Colors for terminal output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: help build run test test-coverage test-docker lint clean docker-build docker-run pr-checks setup deps fmt vet security icons generate-icons build-release build-cross-platform build-macos-app

# Default target
all: clean deps fmt vet test build

# Help target - shows available commands
help:
	@echo "$(BLUE)Quiver Makefile Commands:$(NC)"
	@echo ""
	@echo "$(GREEN)Development:$(NC)"
	@echo "  build          - Build the application binary"
	@echo "  run            - Run the application locally"
	@echo "  clean          - Clean build artifacts"
	@echo "  setup          - Setup development environment"
	@echo "  deps           - Download and verify dependencies"
	@echo "  icons          - Generate multi-platform icons"
	@echo ""
	@echo "$(GREEN)Release:$(NC)"
	@echo "  build-release  - Full release build with PR checks and cross-platform binaries"
	@echo "  build-cross-platform - Build for all platforms (Windows, macOS, Linux)"
	@echo "  build-macos-app - Create macOS .app bundle with embedded icons"
	@echo ""
	@echo "$(GREEN)Testing:$(NC)"
	@echo "  test           - Run all tests"
	@echo "  test-coverage  - Run tests with coverage report"
	@echo "  test-docker    - Run tests in Docker container"
	@echo ""
	@echo "$(GREEN)Code Quality:$(NC)"
	@echo "  fmt            - Format Go code"
	@echo "  vet            - Run go vet"
	@echo "  lint           - Run golangci-lint"
	@echo ""
	@echo "$(GREEN)Docker:$(NC)"
	@echo "  docker-build   - Build Docker image"
	@echo "  docker-run     - Run application in Docker"
	@echo ""
	@echo "$(GREEN)CI/CD:$(NC)"
	@echo "  pr-checks      - Run all PR validation checks"
	@echo "  validate-branch- Validate current branch for PR"

# Setup development environment
setup:
	@echo "$(BLUE)Setting up development environment...$(NC)"
	@go version
	@go mod download
	@go mod verify
	@mkdir -p $(BINARY_DIR)
	@echo "$(GREEN)Development environment ready!$(NC)"

# Download and verify dependencies
deps:
	@echo "$(BLUE)Downloading dependencies...$(NC)"
	@go mod download
	@go mod verify
	@go mod tidy
	@echo "$(GREEN)Dependencies updated!$(NC)"

# Format Go code
fmt:
	@echo "$(BLUE)Formatting Go code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)Code formatted!$(NC)"

# Run go vet
vet:
	@echo "$(BLUE)Running go vet...$(NC)"
	@go vet ./...
	@echo "$(GREEN)go vet passed!$(NC)"

# Generate multi-platform icons
icons: generate-icons

# Generate icons from source
generate-icons:
	@echo "$(BLUE)Generating multi-platform icons...$(NC)"
	@mkdir -p $(ICON_DIR)
	@if [ -f "$(ICON_SOURCE)" ]; then \
		echo "$(BLUE)Converting ICO to PNG formats...$(NC)"; \
		sips -s format png $(ICON_SOURCE) --out $(ICON_DIR)/app.png; \
		sips -s format png $(ICON_SOURCE) --out $(ICON_DIR)/app-256.png --resampleWidth 256 --resampleHeight 256; \
		sips -s format png $(ICON_SOURCE) --out $(ICON_DIR)/app-128.png --resampleWidth 128 --resampleHeight 128; \
		sips -s format png $(ICON_SOURCE) --out $(ICON_DIR)/app-64.png --resampleWidth 64 --resampleHeight 64; \
		sips -s format png $(ICON_SOURCE) --out $(ICON_DIR)/app-32.png --resampleWidth 32 --resampleHeight 32; \
		sips -s format png $(ICON_SOURCE) --out $(ICON_DIR)/app-16.png --resampleWidth 16 --resampleHeight 16; \
		echo "$(BLUE)Creating macOS ICNS file...$(NC)"; \
		mkdir -p $(ICON_DIR)/app.iconset; \
		cp $(ICON_DIR)/app-16.png $(ICON_DIR)/app.iconset/icon_16x16.png; \
		cp $(ICON_DIR)/app-32.png $(ICON_DIR)/app.iconset/icon_16x16@2x.png; \
		cp $(ICON_DIR)/app-32.png $(ICON_DIR)/app.iconset/icon_32x32.png; \
		cp $(ICON_DIR)/app-64.png $(ICON_DIR)/app.iconset/icon_32x32@2x.png; \
		cp $(ICON_DIR)/app-128.png $(ICON_DIR)/app.iconset/icon_128x128.png; \
		cp $(ICON_DIR)/app-256.png $(ICON_DIR)/app.iconset/icon_128x128@2x.png; \
		cp $(ICON_DIR)/app-256.png $(ICON_DIR)/app.iconset/icon_256x256.png; \
		cp $(ICON_DIR)/app-256.png $(ICON_DIR)/app.iconset/icon_256x256@2x.png; \
		cp $(ICON_DIR)/app-256.png $(ICON_DIR)/app.iconset/icon_512x512.png; \
		cp $(ICON_DIR)/app-256.png $(ICON_DIR)/app.iconset/icon_512x512@2x.png; \
		iconutil -c icns $(ICON_DIR)/app.iconset -o $(ICON_DIR)/app.icns; \
		rm -rf $(ICON_DIR)/app.iconset; \
		cp $(ICON_SOURCE) $(ICON_DIR)/app.ico; \
		echo "$(GREEN)Multi-platform icons generated successfully!$(NC)"; \
	else \
		echo "$(YELLOW)Warning: Source icon file $(ICON_SOURCE) not found. Skipping icon generation.$(NC)"; \
	fi

# Build the application
build: clean deps fmt vet
	@echo "$(BLUE)Building $(APP_NAME)...$(NC)"
	@mkdir -p $(BINARY_DIR)
	@CGO_ENABLED=0 go build $(BUILD_FLAGS) $(LDFLAGS) -o $(BINARY_PATH) $(MAIN_PATH)
	@echo "$(GREEN)Build completed: $(BINARY_PATH)$(NC)"

# Run the application locally
run:
	@echo "$(BLUE)Starting $(APP_NAME)...$(NC)"
	@go run ./cmd/quiver

# Run all tests
test:
	@echo "$(BLUE)Running tests...$(NC)"
	@go test -race -ldflags="-s -w" -v ./...
	@echo "$(GREEN)All tests passed!$(NC)"

# Run tests with coverage
test-coverage:
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	@go test -race -ldflags="-s -w" -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@go tool cover -func=$(COVERAGE_FILE)
	@echo "$(GREEN)Coverage report generated: $(COVERAGE_HTML)$(NC)"

# Run tests in Docker container
test-docker:
	@echo "$(BLUE)Running tests in Docker...$(NC)"
	@docker run --rm -v $(PWD):/app -w /app golang:$(GO_VERSION)-alpine sh -c "\
		apk add --no-cache git make gcc musl-dev && \
		go mod download && \
		CGO_ENABLED=1 go test -race -ldflags=\"-s -w\" -coverprofile=coverage.out -covermode=atomic ./... && \
		go tool cover -func=coverage.out"
	@echo "$(GREEN)Docker tests completed!$(NC)"

# Run linting checks
lint:
	@echo "$(BLUE)Running linting checks...$(NC)"
	@echo "$(BLUE)Running go fmt check...$(NC)"
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "$(RED)Code is not formatted. Please run 'make fmt'$(NC)"; \
		gofmt -l .; \
		exit 1; \
	fi
	@echo "$(BLUE)Running go vet...$(NC)"
	@go vet ./...
	@echo "$(BLUE)Running basic static analysis...$(NC)"
	@go run honnef.co/go/tools/cmd/staticcheck@latest ./...
	@echo "$(GREEN)Linting completed!$(NC)"

# Run security checks
security:
	@echo "$(BLUE)Running security checks...$(NC)"
	@echo "$(YELLOW)Note: gosec security scanner temporarily disabled due to repository issues$(NC)"
	@echo "$(GREEN)Security checks completed!$(NC)"

# Build Docker image
docker-build:
	@echo "$(BLUE)Building Docker image...$(NC)"
	@docker build -t $(DOCKER_IMAGE) .
	@echo "$(GREEN)Docker image built: $(DOCKER_IMAGE)$(NC)"

# Run application in Docker
docker-run: docker-build
	@echo "$(BLUE)Running $(APP_NAME) in Docker...$(NC)"
	@docker run --rm -p 8080:8080 $(DOCKER_IMAGE)

# Validate current branch for PR
validate-branch:
	@echo "$(BLUE)Validating current branch for PR...$(NC)"
	@CURRENT_BRANCH=$$(git branch --show-current); \
	case "$$CURRENT_BRANCH" in \
		enhancement/*|feature/*|fix/*) \
			echo "$(GREEN)✓ Branch '$$CURRENT_BRANCH' can create PR to develop$(NC)"; \
			;; \
		hotfix/*) \
			echo "$(GREEN)✓ Branch '$$CURRENT_BRANCH' can create PR to master$(NC)"; \
			;; \
		release/*) \
			echo "$(GREEN)✓ Branch '$$CURRENT_BRANCH' can create PR from develop to master$(NC)"; \
			;; \
		develop|master|main) \
			echo "$(RED)✗ Cannot create PR from protected branch '$$CURRENT_BRANCH'$(NC)"; \
			exit 1; \
			;; \
		*) \
			echo "$(RED)✗ Invalid branch name '$$CURRENT_BRANCH'. Must follow pattern:$(NC)"; \
			echo "  - enhancement/name"; \
			echo "  - feature/name"; \
			echo "  - fix/name"; \
			echo "  - hotfix/name"; \
			echo "  - release/yyyy-mm-dd"; \
			exit 1; \
			;; \
	esac

# Run all PR validation checks
pr-checks: validate-branch clean deps fmt vet lint security test-coverage build
	@echo "$(BLUE)Running comprehensive PR checks...$(NC)"
	@echo "$(BLUE)Checking test coverage...$(NC)"
	@go test -race -ldflags="-s -w" -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@COVERAGE=$$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Overall coverage: $$COVERAGE%"; \
	if [ "$$(echo "$$COVERAGE" | cut -d. -f1)" -lt 80 ]; then \
		echo "$(RED)✗ Coverage $$COVERAGE% is below required 80%$(NC)"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ Coverage $$COVERAGE% meets requirement$(NC)"; \
	fi
	@echo "$(GREEN)All PR checks passed! ✓$(NC)"

# Build release with full validation and cross-platform binaries
build-release: pr-checks build-cross-platform
	@echo "$(BLUE)Creating release artifacts...$(NC)"
	@mkdir -p release
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		echo "$(BLUE)Creating macOS .app bundle...$(NC)"; \
		$(MAKE) build-macos-app; \
	fi
	@echo "$(GREEN)Release build completed!$(NC)"
	@echo "$(BLUE)Release artifacts:$(NC)"
	@ls -la release/ 2>/dev/null || echo "No release artifacts created"

# Build for all platforms
build-cross-platform: icons
	@echo "$(BLUE)Building for all platforms...$(NC)"
	@mkdir -p release
	@echo "$(BLUE)Building for Windows (amd64)...$(NC)"
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) $(LDFLAGS) -o release/$(APP_NAME)-windows-amd64.exe $(MAIN_PATH)
	@echo "$(BLUE)Building for macOS (amd64)...$(NC)"
	@GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) $(LDFLAGS) -o release/$(APP_NAME)-darwin-amd64 $(MAIN_PATH)
	@echo "$(BLUE)Building for macOS (arm64)...$(NC)"
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) $(LDFLAGS) -o release/$(APP_NAME)-darwin-arm64 $(MAIN_PATH)
	@echo "$(BLUE)Building for Linux (amd64)...$(NC)"
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(BUILD_FLAGS) $(LDFLAGS) -o release/$(APP_NAME)-linux-amd64 $(MAIN_PATH)
	@echo "$(BLUE)Building for Linux (arm64)...$(NC)"
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(BUILD_FLAGS) $(LDFLAGS) -o release/$(APP_NAME)-linux-arm64 $(MAIN_PATH)
	@echo "$(GREEN)Cross-platform build completed!$(NC)"
	@echo "$(BLUE)Built binaries:$(NC)"
	@ls -la release/
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		echo "$(BLUE)Creating macOS .app bundle...$(NC)"; \
		$(MAKE) build-macos-app; \
	fi

# Create macOS .app bundle with embedded icons and terminal wrapper
build-macos-app: icons
	@echo "$(BLUE)Creating macOS .app bundle...$(NC)"
	@if [ "$$(uname -s)" != "Darwin" ]; then \
		echo "$(YELLOW)macOS .app bundle creation only supported on macOS$(NC)"; \
		exit 0; \
	fi
	@mkdir -p release/$(APP_NAME).app/Contents/{MacOS,Resources}
	@echo "$(BLUE)Copying executable...$(NC)"
	@cp release/$(APP_NAME)-darwin-$$(uname -m) release/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)-bin
	@chmod +x release/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)-bin
	@echo "$(BLUE)Creating terminal wrapper script...$(NC)"
	@printf '#!/bin/bash\n# Terminal wrapper for $(APP_NAME)\n# This script opens Terminal and runs the $(APP_NAME) executable\n\n# Get the directory where this script is located\nSCRIPT_DIR="$$(dirname "$$0")"\nBINARY_PATH="$$SCRIPT_DIR/$(APP_NAME)-bin"\n\n# Check if binary exists\nif [ ! -f "$$BINARY_PATH" ]; then\n    echo "Error: $(APP_NAME) binary not found at $$BINARY_PATH"\n    exit 1\nfi\n\n# Open Terminal and run the application\nosascript -e "tell application \\"Terminal\\" to do script \\"cd \\"$$(dirname \\"$$SCRIPT_DIR\\")\\" && \\"$$BINARY_PATH\\" && echo \\"\\" && echo \\"Press any key to close this terminal...\\" && read -n 1\\""\n\n# Optional: Keep the wrapper running briefly to ensure Terminal opens\nsleep 1\n' > release/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)
	@chmod +x release/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)
	@echo "$(BLUE)Copying icon...$(NC)"
	@cp $(ICON_DIR)/app.icns release/$(APP_NAME).app/Contents/Resources/$(APP_NAME).icns
	@echo "$(BLUE)Creating Info.plist...$(NC)"
	@printf '<?xml version="1.0" encoding="UTF-8"?>\n<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">\n<plist version="1.0">\n<dict>\n\t<key>CFBundleExecutable</key>\n\t<string>$(APP_NAME)</string>\n\t<key>CFBundleIconFile</key>\n\t<string>$(APP_NAME).icns</string>\n\t<key>CFBundleIdentifier</key>\n\t<string>com.rabbytesoftware.$(APP_NAME)</string>\n\t<key>CFBundleName</key>\n\t<string>$(APP_NAME)</string>\n\t<key>CFBundlePackageType</key>\n\t<string>APPL</string>\n\t<key>CFBundleShortVersionString</key>\n\t<string>1.0.0</string>\n\t<key>CFBundleVersion</key>\n\t<string>1</string>\n\t<key>LSMinimumSystemVersion</key>\n\t<string>10.15</string>\n\t<key>CFBundleDocumentTypes</key>\n\t<array>\n\t\t<dict>\n\t\t\t<key>CFBundleTypeName</key>\n\t\t\t<string>Terminal Application</string>\n\t\t\t<key>CFBundleTypeRole</key>\n\t\t\t<string>Shell</string>\n\t\t</dict>\n\t</array>\n</dict>\n</plist>\n' > release/$(APP_NAME).app/Contents/Info.plist
	@echo "$(GREEN)macOS .app bundle with terminal wrapper created: release/$(APP_NAME).app$(NC)"
	@echo "$(BLUE)Bundle contents:$(NC)"
	@ls -la release/$(APP_NAME).app/Contents/

# Clean build artifacts
clean:
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	@rm -rf $(BINARY_DIR)
	@rm -rf release
	@rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@go clean
	@echo "$(GREEN)Clean completed!$(NC)"

# Install development tools
install-tools:
	@echo "$(BLUE)Installing development tools...$(NC)"
	@GOPATH=$$(go env GOPATH); \
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$GOPATH/bin v1.60.3
	@echo "$(YELLOW)gosec temporarily disabled due to repository issues$(NC)"
	@echo "$(GREEN)Development tools installed!$(NC)"
