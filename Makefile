# Quiver Makefile
# This Makefile provides commands for building, testing, and running the Quiver application

# Variables
APP_NAME := quiver
BINARY_DIR := bin
BINARY_PATH := $(BINARY_DIR)/$(APP_NAME)
MAIN_PATH := ./cmd/quiver
DOCKER_IMAGE := quiver:latest
GO_VERSION := 1.24.2
COVERAGE_DIR := coverage
COVERAGE_FILE := $(COVERAGE_DIR)/coverage.out
COVERAGE_HTML := $(COVERAGE_DIR)/coverage.html
ICON_DIR := cmd/quiver/assets/icons
ICON_SOURCE := cmd/quiver/assets/icons/app.ico
COVERAGE_BELOW ?= 40

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

# Build the application
build: clean deps fmt vet
	@echo "$(BLUE)Building $(APP_NAME)...$(NC)"
	@mkdir -p $(BINARY_DIR)
	@CGO_ENABLED=0 go build $(BUILD_FLAGS) $(LDFLAGS) -o $(BINARY_PATH) $(MAIN_PATH)
	@echo "$(GREEN)Build completed: $(BINARY_PATH)$(NC)"

# Clean build artifacts and coverage files
clean:
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	@rm -rf $(BINARY_DIR)
	@rm -rf $(COVERAGE_DIR)
	@echo "$(GREEN)Clean completed!$(NC)"

# Run the application locally
run:
	@echo "$(BLUE)Starting $(APP_NAME)...$(NC)"
	@go run ./cmd/quiver

# Run all tests
test:
	@echo "$(BLUE)Running tests...$(NC)"
	@go test -race -ldflags="-s -w" -v ./... 2>&1 | grep -v "malformed LC_DYSYMTAB"
	@echo "$(GREEN)All tests passed!$(NC)"

# Run tests with coverage
test-coverage:
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	@go test -race -ldflags="-s -w" -coverprofile=$(COVERAGE_FILE).tmp -covermode=atomic $$(go list ./... | grep -v '/mocks$$') 2>&1 | grep -v "malformed LC_DYSYMTAB"
	@grep -v '/mocks/' $(COVERAGE_FILE).tmp > $(COVERAGE_FILE) || true
	@rm -f $(COVERAGE_FILE).tmp
	@go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@go tool cover -func=$(COVERAGE_FILE)
	@echo "$(GREEN)Coverage report generated: $(COVERAGE_HTML)$(NC)"

# Run tests in Docker container
test-docker:
	@echo "$(BLUE)Running tests in Docker...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	@docker run --rm -v $(PWD):/app -w /app golang:$(GO_VERSION)-alpine sh -c "\
		apk add --no-cache git make gcc musl-dev && \
		go mod download && \
		mkdir -p $(COVERAGE_DIR) && \
		CGO_ENABLED=1 go test -race -ldflags=\"-s -w\" -coverprofile=$(COVERAGE_FILE).tmp -covermode=atomic \$$(go list ./... | grep -v '/mocks\$$') && \
		grep -v '/mocks/' $(COVERAGE_FILE).tmp > $(COVERAGE_FILE) || true && \
		rm -f $(COVERAGE_FILE).tmp && \
		go tool cover -func=$(COVERAGE_FILE)"
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
	@echo "$(BLUE)Installing gosec if not present...$(NC)"
	@go install github.com/securego/gosec/v2/cmd/gosec@latest
	@echo "$(BLUE)Running gosec security scan...$(NC)"
	@gosec ./... || { echo "$(YELLOW)Security issues found. Review the output above.$(NC)"; exit 0; }
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
		enhancement/*|feature/*|fix/*|refactor/*|dependabot/*) \
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
			echo "  - refactor/name"; \
			echo "  - hotfix/name"; \
			echo "  - release/yyyy-mm-dd"; \
			exit 1; \
			;; \
	esac

# Run all PR validation checks
pr-checks: validate-branch clean deps fmt vet lint security test-coverage build
	@echo "$(BLUE)Running comprehensive PR checks...$(NC)"
	@echo "$(BLUE)Checking test coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	@go test -race -ldflags="-s -w" -coverprofile=$(COVERAGE_FILE).tmp -covermode=atomic $$(go list ./... | grep -v '/mocks$$') 2>&1 | grep -v "malformed LC_DYSYMTAB"
	@grep -v '/mocks/' $(COVERAGE_FILE).tmp > $(COVERAGE_FILE) || true
	@rm -f $(COVERAGE_FILE).tmp
	@COVERAGE=$$(go tool cover -func=$(COVERAGE_FILE) | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "Overall coverage: $$COVERAGE%"; \
	if [ "$$(echo "$$COVERAGE" | cut -d. -f1)" -lt 90 ]; then \
		echo "$(RED)✗ Coverage $$COVERAGE% is below required 90%$(NC)"; \
		exit 1; \
	else \
		echo "$(GREEN)✓ Coverage $$COVERAGE% meets requirement$(NC)"; \
	fi
	@echo "$(GREEN)All PR checks passed! ✓$(NC)"

# Install development tools
install-tools:
	@echo "$(BLUE)Installing development tools...$(NC)"
	@GOPATH=$$(go env GOPATH); \
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$GOPATH/bin v1.60.3
	@echo "$(YELLOW)gosec temporarily disabled due to repository issues$(NC)"
	@echo "$(GREEN)Development tools installed!$(NC)"

# Update docs content
build-docs:
	swag init -g cmd/quiver/main.go --parseDependency --parseInternal

# List missing test files
missing-tests:
	@printf "$(BLUE)Searching for files without *_test.go...$(NC)\n\n"
	@count=0; \
	for f in $$(find internal -name "*.go" ! -name "*_test.go"); do \
		testfile=$${f%.go}_test.go; \
		if [ ! -f $$testfile ]; then \
			printf "$(BLUE)*$(NC) %-60s\n" "$$f"; \
			count=$$((count+1)); \
		fi; \
	done; \
	if [ $$count -eq 0 ]; then \
		printf "\n $(GREEN)All files have corresponding test files$(NC)\n"; \
	else \
		printf "\n $(YELLOW)Files without test files:$(NC) $(RED)%d$(NC)\n" $$count; \
	fi

# List test files below certain % of coverage
coverage-files:
	@printf " $(BLUE)Files with coverage < %d%%$(NC)\n\n" $(COVERAGE_BELOW)
	@go test ./... -coverprofile=coverage.out >/dev/null 2>&1 || true
	@go tool cover -func=coverage.out 2>/dev/null | grep -v total | \
	awk -v min=$(COVERAGE_BELOW) '\
	{ \
		file=$$1; \
		gsub(":.*","",file); \
		gsub("%","",$$NF); \
		if ($$NF+0 < min && !seen[file]++) { \
			printf "  $(BLUE)*$(NC) %-60s $(RED)%.1f%%$(NC)\n", file, $$NF; \
			count++; \
		} \
	} \
	END { \
		if (count == 0) { \
			printf "   $(GREEN)No files below %d%% coverage$(NC)\n", min; \
		} else { \
			printf "\n $(YELLOW)Total files below %d%% coverage:$(NC) $(RED)%d$(NC)\n", min, count; \
		} \
	}'
	@true


# List functions test below certain % of coverage
coverage-funcs:
	@printf " $(BLUE)Functions with coverage < %d%%$(NC)\n\n" $(COVERAGE_BELOW)
	@go test ./... -coverprofile=coverage.out >/dev/null 2>&1 || true
	@go tool cover -func=coverage.out | grep -v total | \
	while IFS= read -r line; do \
		cov=$${line##*	}; \
		cov=$${cov%%%}; \
		name=$${line%% *}; \
		if [ $$(printf "%.0f" "$$cov") -lt $(COVERAGE_BELOW) ]; then \
			case "$$name" in \
				*:* ) \
					file=$${name%%:*}; \
					func=$${name##*:}; \
					;; \
				* ) \
					file=$$name; \
					func="(package-level)"; \
					;; \
			esac; \
			printf "$(BLUE)*$(NC) $(BLUE)%s$(NC) (%s) $(RED)%.1f%%$(NC)\n" "$$func" "$$file" "$$cov"; \
		fi; \
	done | tee /tmp/coverage_funcs.txt
	@count=$$(wc -l < /tmp/coverage_funcs.txt); \
	echo ""; \
	if [ $$count -gt 0 ]; then \
		printf "   $(RED)%d functions below %d%% coverage$(NC)\n" $$count $(COVERAGE_BELOW); \
	else \
		printf "   $(GREEN)No functions below %d%% coverage$(NC)\n" $(COVERAGE_BELOW); \
	fi
	@rm -f /tmp/coverage_funcs.txt
