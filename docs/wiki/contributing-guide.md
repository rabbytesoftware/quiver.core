# Contributing Guide

Thank you for your interest in contributing to Quiver! This guide will help you get started with contributing to the project.

## Table of Contents

- [Getting Started](#getting-started)
- [Development Process](#development-process)
- [Code Standards](#code-standards)
- [Testing Requirements](#testing-requirements)
- [Pull Request Process](#pull-request-process)
- [Issue Reporting](#issue-reporting)
- [Community Guidelines](#community-guidelines)

## Getting Started

### Prerequisites

Before contributing, ensure you have:

1. **Go 1.24.2 or later** - [Download from golang.org](https://golang.org/dl/)
2. **Docker** - [Download from docker.com](https://www.docker.com/get-started)
3. **Make** - Usually pre-installed on macOS/Linux
4. **Git** - [Download from git-scm.com](https://git-scm.com/downloads)

### Fork and Clone

1. **Fork the repository** on GitHub
2. **Clone your fork**:
   ```bash
   git clone https://github.com/YOUR_USERNAME/quiver.git
   cd quiver
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/rabbytesoftware/quiver.git
   ```

### Setup Development Environment

```bash
# Setup the development environment
make setup

# Verify everything works
make test
make build
```

## Development Process

### 1. Choose Your Contribution Type

#### Bug Fixes
- **Branch**: `fix/description-of-fix`
- **Target**: `develop`
- **Examples**: `fix/memory-leak-issue`, `fix/api-validation-bug`

#### New Features
- **Branch**: `feature/description-of-feature`
- **Target**: `develop`
- **Examples**: `feature/user-authentication`, `feature/package-manager`

#### Enhancements
- **Branch**: `enhancement/description-of-enhancement`
- **Target**: `develop`
- **Examples**: `enhancement/api-performance`, `enhancement/ui-improvements`

#### Hotfixes
- **Branch**: `hotfix/description-of-hotfix`
- **Target**: `master`
- **Examples**: `hotfix/security-vulnerability`, `hotfix/critical-bug`

### 2. Create Your Branch

```bash
# Update your local develop branch
git checkout develop
git pull upstream develop

# Create your feature branch
git checkout -b feature/your-feature-name
```

### 3. Make Your Changes

#### Code Organization
- Follow the existing project structure
- Place code in appropriate packages
- Keep functions focused and single-purpose
- Use meaningful variable and function names

#### File Size Guidelines
- **Maximum file size**: 500 lines
- **If larger**: Split into multiple files within the same package
- **Consider**: Breaking into separate packages if functionality grows

#### Import Organization
```go
// Standard library imports
import (
    "context"
    "fmt"
    "time"
)

// Third-party imports
import (
    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
)

// Internal imports
import (
    "github.com/rabbytesoftware/quiver/internal/models"
    "github.com/rabbytesoftware/quiver/internal/repositories"
)
```

### 4. Test Your Changes

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests in Docker (matches CI)
make test-docker

# Run linting
make lint

# Run security checks
make security
```

### 5. Validate Before PR

```bash
# Run all PR validation checks
make pr-checks

# Validate branch naming
make validate-branch
```

## Code Standards

### Go Conventions

#### Naming Conventions
- **Packages**: lowercase, single word (e.g., `models`, `api`)
- **Functions**: camelCase for private, PascalCase for public
- **Variables**: camelCase for private, PascalCase for public
- **Constants**: PascalCase or UPPER_CASE
- **Interfaces**: PascalCase, often ending with "er" (e.g., `Reader`, `Writer`)

#### Code Style
```go
// Good: Clear, descriptive names
func CreateArrow(name string, version string) (*Arrow, error) {
    // Implementation
}

// Good: Proper error handling
result, err := processData(input)
if err != nil {
    return nil, fmt.Errorf("failed to process data: %w", err)
}

// Good: Context usage for cancelable operations
func ProcessWithContext(ctx context.Context, data []byte) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        // Process data
    }
}
```

#### Error Handling
```go
// Always handle errors
result, err := someOperation()
if err != nil {
    return fmt.Errorf("operation failed: %w", err)
}

// Use wrapped errors for context
if err != nil {
    return fmt.Errorf("failed to create arrow %s: %w", arrowName, err)
}
```

### Documentation Standards

#### Code Comments
```go
// Package models provides domain entities for the Quiver system.
package models

// Arrow represents a game server package that can be installed and run.
// It contains all necessary information for package management including
// metadata, requirements, dependencies, and execution methods.
type Arrow struct {
    // ID is the unique identifier for this arrow
    ID uuid.UUID `json:"id"`
    
    // Name is the human-readable name of the arrow
    Name string `json:"name"`
    
    // ... other fields
}
```

#### Function Documentation
```go
// CreateArrow creates a new arrow with the specified name and version.
// It validates the input parameters and returns an error if validation fails.
// The created arrow will have a generated UUID and default configuration.
func CreateArrow(name, version string) (*Arrow, error) {
    // Implementation
}
```

### Architecture Guidelines

#### Clean Architecture
- **Dependencies point inward**: API → Usecases → Repositories → Infrastructure
- **Business logic in usecases**: Keep domain logic separate from technical concerns
- **Interface segregation**: Small, focused interfaces
- **Dependency injection**: All dependencies injected through constructors

#### Package Design
```go
// Good: Single responsibility
package arrows

// Good: Clear interface
type ArrowsInterface interface {
    Create(arrow *Arrow) error
    GetByID(id uuid.UUID) (*Arrow, error)
    Update(arrow *Arrow) error
    Delete(id uuid.UUID) error
}
```

## Testing Requirements

### Coverage Requirements
- **Overall Project Coverage**: ≥ 80%
- **Pull Request Coverage**: ≥ 80%

### Test Structure
```
internal/
├── models/
│   ├── arrow/
│   │   ├── arrow.go
│   │   └── arrow_test.go
│   └── quiver/
│       ├── quiver.go
│       └── quiver_test.go
```

### Test Naming
```go
func TestNewArrow(t *testing.T)                    // Constructor tests
func TestArrow_Create(t *testing.T)               // Method tests
func TestArrow_WithInvalidInput(t *testing.T)     // Error case tests
func TestArrow_Integration(t *testing.T)         // Integration tests
```

### Test Examples
```go
func TestCreateArrow(t *testing.T) {
    // Arrange
    name := "test-arrow"
    version := "1.0.0"
    
    // Act
    arrow, err := CreateArrow(name, version)
    
    // Assert
    if err != nil {
        t.Fatalf("Expected no error, got %v", err)
    }
    
    if arrow.Name != name {
        t.Errorf("Expected name %s, got %s", name, arrow.Name)
    }
    
    if arrow.Version != version {
        t.Errorf("Expected version %s, got %s", version, arrow.Version)
    }
}
```

### Integration Tests
```go
func TestArrowRepository_Integration(t *testing.T) {
    // Setup test database
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)
    
    // Create repository
    repo := NewArrowRepository(db)
    
    // Test operations
    arrow := &Arrow{Name: "test", Version: "1.0.0"}
    err := repo.Create(arrow)
    if err != nil {
        t.Fatalf("Failed to create arrow: %v", err)
    }
    
    // Verify creation
    retrieved, err := repo.GetByID(arrow.ID)
    if err != nil {
        t.Fatalf("Failed to retrieve arrow: %v", err)
    }
    
    if retrieved.Name != arrow.Name {
        t.Errorf("Expected name %s, got %s", arrow.Name, retrieved.Name)
    }
}
```

## Pull Request Process

### 1. Before Creating PR

```bash
# Ensure your branch is up-to-date
git checkout develop
git pull upstream develop
git checkout feature/your-feature
git rebase develop

# Run all validation checks
make pr-checks

# Push your branch
git push origin feature/your-feature
```

### 2. Create Pull Request

1. **Go to GitHub** and create a new pull request
2. **Choose the correct target branch**:
   - `feature/*` → `develop`
   - `fix/*` → `develop`
   - `enhancement/*` → `develop`
   - `hotfix/*` → `master`
   - `release/*` → `master`

3. **Fill out the PR template**:
   - Description of changes
   - Testing performed
   - Screenshots (if applicable)
   - Breaking changes (if any)

### 3. PR Template

```markdown
## Description
Brief description of the changes made.

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Enhancement
- [ ] Documentation update
- [ ] Hotfix

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Manual testing performed
- [ ] All tests pass locally

## Checklist
- [ ] Code follows project style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No breaking changes (or documented if necessary)
- [ ] CI checks pass
```

### 4. Review Process

1. **Automated Checks**: CI will run all validation checks
2. **Code Review**: At least one team member will review your code
3. **Address Feedback**: Make requested changes and push updates
4. **Approval**: Once approved, your PR will be merged

## Issue Reporting

### Bug Reports

When reporting bugs, please include:

1. **Clear description** of the issue
2. **Steps to reproduce** the problem
3. **Expected behavior** vs actual behavior
4. **Environment details**:
   - Operating system
   - Go version
   - Quiver version
5. **Logs and error messages** (if applicable)
6. **Screenshots** (if applicable)

### Feature Requests

When requesting features, please include:

1. **Clear description** of the feature
2. **Use case** and motivation
3. **Proposed implementation** (if you have ideas)
4. **Alternative solutions** considered
5. **Additional context** and examples

### Issue Template

```markdown
## Description
Brief description of the issue or feature request.

## Steps to Reproduce (for bugs)
1. Go to '...'
2. Click on '....'
3. Scroll down to '....'
4. See error

## Expected Behavior
What you expected to happen.

## Actual Behavior
What actually happened.

## Environment
- OS: [e.g. macOS, Linux, Windows]
- Go Version: [e.g. 1.24.2]
- Quiver Version: [e.g. 1.0.0]

## Additional Context
Any other context about the problem or feature request.
```

## Community Guidelines

### Code of Conduct

We are committed to providing a welcoming and inclusive environment for all contributors. Please:

1. **Be respectful** and constructive in all interactions
2. **Be patient** with newcomers and help them learn
3. **Be collaborative** and open to different approaches
4. **Be professional** in all communications
5. **Be inclusive** and welcoming to all contributors

### Communication

- **GitHub Issues**: For bug reports and feature requests
- **GitHub Discussions**: For general questions and ideas
- **Pull Request Comments**: For code review and feedback
- **Commit Messages**: Use clear, descriptive messages

### Getting Help

1. **Check existing issues** and discussions first
2. **Read the documentation** thoroughly
3. **Ask specific questions** with context
4. **Provide examples** when possible
5. **Be patient** for responses

## Development Tips

### Local Development

```bash
# Run the application locally
make run

# Run tests during development
make test

# Check code formatting
make fmt

# Run linting
make lint
```

### Debugging

```bash
# Run with debug logging
QUIVER_LOG_LEVEL=debug make run

# Run tests with verbose output
go test -v ./...

# Run specific test
go test -v ./internal/models/arrow
```

### Performance

```bash
# Profile the application
go run -cpuprofile=cpu.prof ./cmd/quiver
go tool pprof cpu.prof

# Memory profiling
go run -memprofile=mem.prof ./cmd/quiver
go tool pprof mem.prof
```

## Release Process

### Version Numbering

We follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Incompatible API changes
- **MINOR**: New functionality in a backwards compatible manner
- **PATCH**: Backwards compatible bug fixes

### Release Checklist

1. **Update version numbers** in relevant files
2. **Update changelog** with new features and fixes
3. **Run full test suite** to ensure stability
4. **Update documentation** if needed
5. **Create release notes** for users

## Recognition

Contributors will be recognized in:
- **CONTRIBUTORS.md** file
- **Release notes** for significant contributions
- **GitHub contributors** page
- **Project documentation** where appropriate

## Questions?

If you have questions about contributing:

1. **Check the documentation** first
2. **Search existing issues** and discussions
3. **Create a new issue** with your question
4. **Join discussions** in GitHub Discussions

Thank you for contributing to Quiver! 🎉

---

*For more information about the development process, see the [Git Workflow](git-workflow.md) and [Development Setup](development-setup.md) documentation.*
