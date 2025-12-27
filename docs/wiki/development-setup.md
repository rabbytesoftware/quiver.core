# Development Setup

This guide will help you set up your development environment for the Quiver package manager project.

## Prerequisites

Before you begin, ensure you have the following installed:

### Required Software

- **Go 1.24.2 or later** - [Download from golang.org](https://golang.org/dl/)
- **Docker** - [Download from docker.com](https://www.docker.com/get-started)
- **Make** - Usually pre-installed on macOS/Linux, [Windows users](https://www.gnu.org/software/make/)
- **Git** - [Download from git-scm.com](https://git-scm.com/downloads)

### Optional but Recommended

- **VS Code** with Go extension
- **GoLand** or **IntelliJ IDEA** with Go plugin
- **Terminal** with good Go support (iTerm2, Windows Terminal, etc.)

## Initial Setup

### 1. Clone the Repository

```bash
git clone https://github.com/rabbytesoftware/quiver.git
cd quiver
```

### 2. Verify Prerequisites

```bash
# Check Go version
go version
# Should show: go version go1.24.2 or later

# Check Docker
docker --version
# Should show: Docker version 20.x.x or later

# Check Make
make --version
# Should show: GNU Make 4.x or later
```

### 3. Setup Development Environment

```bash
# Run the setup command (this will download dependencies and create necessary directories)
make setup
```

This command will:
- Download and verify Go dependencies
- Create necessary directories (`bin/`, `logs/`)
- Set up the development environment

### 4. Verify Installation

```bash
# Build the project
make build

# Run tests to ensure everything works
make test

# Check that the binary was created
ls -la bin/
# Should show: quiver executable
```

## Development Workflow

### Daily Development Commands

```bash
# Run the application locally
make run

# Run tests during development
make test

# Run tests with coverage
make test-coverage

# Format code
make fmt

# Run linting
make lint

# Clean build artifacts
make clean
```

### Before Creating Pull Requests

```bash
# Run all PR validation checks locally
make pr-checks

# This includes:
# - Branch validation
# - Code formatting
# - Linting
# - Security checks
# - Tests with coverage
# - Build verification
```

## Configuration

### Default Configuration

The application uses a default configuration file located at `internal/core/config/default.yaml`:

```yaml
config:
  netbridge:
    enabled: true
    allowed_ports: "40128-40256"

  arrows:
    repositories:
      - ./pkgs
      - https://raw.githubusercontent.com/rabbytesoftware/quiver.arrows/main
    install_dir: ./arrows

  api:
    host: 0.0.0.0
    port: 40257

  database:
    path: ./.db

  watcher:
    enabled: true
    level: info
    folder: ./logs
    max_size: 100
    max_age: 7
    compress: true
```

### Environment Variables

You can override configuration using environment variables:

```bash
# Override API port
export QUIVER_API_PORT=8080

# Override log level
export QUIVER_LOG_LEVEL=debug

# Override database path
export QUIVER_DB_PATH=/tmp/quiver.db
```

## Project Structure

After setup, your project should look like this:

```
quiver/
├── bin/                    # Compiled binaries
├── cmd/                    # Main applications
│   └── quiver/            # Main Quiver application
├── docs/                   # Documentation (this wiki)
├── internal/               # Private application code
│   ├── api/               # REST API layer
│   ├── core/              # Core services
│   ├── infrastructure/    # Infrastructure layer
│   ├── models/            # Domain models
│   ├── repositories/      # Data access layer
│   └── usecases/          # Business logic layer
├── logs/                   # Application logs
├── arrow.dev/             # Arrow package examples
├── Makefile               # Build and development commands
├── go.mod                 # Go module dependencies
└── README.md              # Project overview
```

## Running the Application

### Development Mode

```bash
# Start the application with development settings
make run
```

This will:
- Build the application
- Start the REST API server on `http://localhost:40257`
- Launch the Terminal UI
- Enable debug logging

### API Endpoints

Once running, you can access:

- **Health Check**: `GET http://localhost:40257/api/v1/health`
- **API Documentation**: Available in the [REST API Documentation](rest-api.md)

### Terminal UI

The application provides a Terminal UI with the following features:

- **Real-time Logs**: View application logs in real-time
- **Command Interface**: Execute commands for package management
- **Status Monitoring**: Monitor application status and health

Common commands:
- `help` - Show available commands
- `status` - Show application status
- `quit` or `q` - Exit the application

## Testing

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run tests in Docker (matches CI environment)
make test-docker
```

### Coverage Requirements

- **Overall Project Coverage**: ≥ 80%
- **Pull Request Coverage**: ≥ 80%

### Test Structure

```
internal/
├── api/
│   ├── api_test.go
│   └── v1/
│       └── controllers/
│           └── health/
│               └── health_test.go
├── core/
│   ├── core_test.go
│   └── config/
│       └── config_test.go
└── models/
    ├── arrow/
    │   └── arrow_test.go
    └── quiver/
        └── quiver_test.go
```

## Docker Development

### Running in Docker

```bash
# Build Docker image
make docker-build

# Run in Docker
make docker-run
```

### Testing in Docker

```bash
# Run tests in Docker container (matches CI)
make test-docker
```

## Troubleshooting

### Common Issues

#### Go Version Issues
```bash
# If you get Go version errors
go version
# Ensure you have Go 1.24.2 or later
```

#### Permission Issues
```bash
# If you get permission errors on macOS/Linux
chmod +x bin/quiver
```

#### Port Already in Use
```bash
# If port 40257 is already in use
lsof -i :40257
# Kill the process or change the port in configuration
```

#### Docker Issues
```bash
# If Docker commands fail
docker --version
# Ensure Docker is running
```

### Getting Help

1. **Check the logs**: Look in `logs/quiver.log` for error messages
2. **Run diagnostics**: Use `make pr-checks` to identify issues
3. **Review configuration**: Check `internal/core/config/default.yaml`
4. **Consult documentation**: See [Troubleshooting Guide](troubleshooting.md)

## Next Steps

After completing setup:

1. **Read [Project Structure](project-structure.md)** - Understand the codebase organization
2. **Study [Architecture Overview](architecture-overview.md)** - Learn the system design
3. **Review [Git Workflow](git-workflow.md)** - Understand the development process
4. **Check [Contributing Guide](contributing-guide.md)** - Learn how to contribute

## Development Tips

### Code Organization
- Follow Go project layout standards
- Keep functions focused and single-purpose
- Use meaningful package names
- Handle errors appropriately

### Testing
- Write tests alongside code
- Aim for high test coverage
- Use descriptive test names
- Test both success and error cases

### Performance
- Use `go vet` and `go fmt` regularly
- Profile code when needed
- Monitor memory usage
- Use appropriate data structures

---

*For questions about setup or development, please check the [FAQ](faq-patterns.md) or create an issue.*
