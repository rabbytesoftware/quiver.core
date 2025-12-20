# REST API Documentation

This document provides comprehensive documentation for Quiver's package manager REST API endpoints, request/response formats, and usage examples.

## API Overview

**Base URL**: `http://localhost:40257/api/v1`
**Content-Type**: `application/json`
**Authentication**: Currently none (future: JWT tokens)

## Base Information

### Health Check

Check if the API is running and healthy.

```http
GET /api/v1/health
```

**Response**:
```json
{
  "message": "Sector 7C"
}
```

**Status Codes**:
- `200 OK` - API is healthy

## Arrow Management

Arrows are packages that can be installed and managed through the Quiver package manager API.

### Search Arrows

Search for available Arrow packages.

```http
GET /api/v1/arrow/search?q={query}&repository={repository}
```

**Parameters**:
- `q` (string, required): Search query
- `repository` (string, optional): Specific repository to search

**Examples**:
```bash
# Search all repositories for "cs2"
GET /api/v1/arrow/search?q=cs2

# Search specific repository
GET /api/v1/arrow/search?q=cs2&repository=github.com/repo
```

**Response**:
```json
{
  "arrows": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "namespace": {
        "namespace": "cs2",
        "name": "server"
      },
      "name": "Counter-Strike 2 Server",
      "description": "A Counter-Strike 2 dedicated server",
      "version": "1.0.0",
      "license": "MIT",
      "maintainers": ["https://char2cs.net"],
      "credits": ["Valve Corporation"],
      "url": "https://github.com/rabbytesoftware/arrow.cs2",
      "documentation": "https://github.com/rabbytesoftware/arrow.cs2",
      "requirements": {
        "cpu_cores": 2,
        "memory": 4,
        "disk": 30,
        "os": "linux"
      },
      "dependencies": [],
      "netbridge": [
        {
          "name": "GAME_PORT",
          "protocol": "tcp/udp",
          "port": 27015,
          "required": true
        }
      ],
      "variables": [
        {
          "name": "SERVER_HOSTNAME",
          "default": "CS2 Server",
          "values": [],
          "min": 0,
          "max": 0,
          "sensitive": false,
          "type": "string"
        }
      ],
      "methods": [
        {
          "platform": "linux",
          "actions": {
            "install": "${steamcmd.execute} +login anonymous +force_install_dir ${INSTALL_PATH} +app_update 730 validate +quit",
            "execute": "${INSTALL_PATH}/cs2 -dedicated -console -usercon +hostname ${SERVER_HOSTNAME} +map ${DEFAULT_MAP} +maxplayers ${MAX_PLAYERS} +sv_password ${SERVER_PASSWORD} -port ${GAME_PORT}"
          }
        }
      ]
    }
  ],
  "total": 1,
  "page": 1,
  "per_page": 20
}
```

### Install Arrow

Install an Arrow package.

```http
POST /api/v1/arrow/{namespace}/install
```

**Path Parameters**:
- `namespace` (string): Arrow namespace (e.g., "cs2", "minecraft")

**Request Body**:
```json
{
  "repository": "github.com/repo",
  "variables": {
    "SERVER_HOSTNAME": "My CS2 Server",
    "MAX_PLAYERS": "16",
    "SERVER_PASSWORD": "mypassword"
  }
}
```

**Query Parameters**:
- `repository` (string, optional): Repository to install from

**Examples**:
```bash
# Install from any repository
curl -X POST http://localhost:40257/api/v1/arrow/cs2/install \
  -H "Content-Type: application/json" \
  -d '{
    "variables": {
      "SERVER_HOSTNAME": "My CS2 Server",
      "MAX_PLAYERS": "16"
    }
  }'

# Install from specific repository
curl -X POST "http://localhost:40257/api/v1/arrow/cs2/install?repository=github.com/repo" \
  -H "Content-Type: application/json" \
  -d '{
    "variables": {
      "SERVER_HOSTNAME": "My CS2 Server"
    }
  }'
```

**Response**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "namespace": "cs2",
  "status": "installing",
  "message": "Arrow installation started",
  "install_path": "./arrows/cs2",
  "estimated_time": "5-10 minutes"
}
```

**Status Codes**:
- `200 OK` - Installation started
- `400 Bad Request` - Invalid request
- `404 Not Found` - Arrow not found
- `409 Conflict` - Arrow already installed
- `500 Internal Server Error` - Installation failed

### Update Arrow

Update an installed Arrow package.

```http
PUT /api/v1/arrow/{namespace}/update
```

**Path Parameters**:
- `namespace` (string): Arrow namespace

**Request Body**:
```json
{
  "repository": "github.com/repo"
}
```

**Query Parameters**:
- `repository` (string, optional): Repository to update from

**Examples**:
```bash
# Update from any repository
curl -X PUT http://localhost:40257/api/v1/arrow/cs2/update

# Update from specific repository
curl -X PUT "http://localhost:40257/api/v1/arrow/cs2/update?repository=github.com/repo"
```

**Response**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "namespace": "cs2",
  "status": "updating",
  "message": "Arrow update started",
  "current_version": "1.0.0",
  "target_version": "1.1.0"
}
```

### Uninstall Arrow

Remove an installed Arrow package.

```http
DELETE /api/v1/arrow/{namespace}
```

**Path Parameters**:
- `namespace` (string): Arrow namespace

**Response**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "namespace": "cs2",
  "status": "uninstalling",
  "message": "Arrow uninstallation started"
}
```

### Get Arrow Status

Get the status of an installed Arrow.

```http
GET /api/v1/arrow/{namespace}
```

**Path Parameters**:
- `namespace` (string): Arrow namespace

**Response**:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "namespace": "cs2",
  "status": "running",
  "version": "1.0.0",
  "install_path": "./arrows/cs2",
  "pid": 12345,
  "uptime": "2h 30m",
  "memory_usage": "512MB",
  "cpu_usage": "15%",
  "ports": [
    {
      "name": "GAME_PORT",
      "port": 27015,
      "protocol": "tcp/udp",
      "status": "open"
    }
  ]
}
```

## Quiver Repository Management

Quivers are repositories where Arrow packages are found and managed.

### List Quiver Repositories

Get a list of all Quiver repositories.

```http
GET /api/v1/quiver
```

**Response**:
```json
{
  "quivers": [
    {
      "id": "quiver-1",
      "name": "My Game Server",
      "description": "Main game server instance",
      "banner": "https://example.com/banner.png",
      "url": "https://example.com",
      "security": "trusted",
      "maintainers": ["admin@example.com"],
      "version": "1.0.0",
      "installed_arrows": [
        {
          "id": "550e8400-e29b-41d4-a716-446655440000",
          "namespace": "cs2",
          "name": "Counter-Strike 2 Server",
          "version": "1.0.0",
          "status": "running"
        }
      ],
      "listed_arrows": [
        {
          "namespace": "minecraft",
          "name": "Minecraft Server"
        }
      ]
    }
  ],
  "total": 1
}
```

### Create Quiver Repository

Create a new Quiver repository.

```http
POST /api/v1/quiver
```

**Request Body**:
```json
{
  "name": "My Game Server",
  "description": "Main game server instance",
  "banner": "https://example.com/banner.png",
  "url": "https://example.com",
  "security": "trusted",
  "maintainers": ["admin@example.com"]
}
```

**Response**:
```json
{
  "id": "quiver-1",
  "name": "My Game Server",
  "description": "Main game server instance",
  "banner": "https://example.com/banner.png",
  "url": "https://example.com",
  "security": "trusted",
  "maintainers": ["admin@example.com"],
  "version": "1.0.0",
  "installed_arrows": [],
  "listed_arrows": [],
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z"
}
```

### Get Quiver Repository

Get details of a specific Quiver repository.

```http
GET /api/v1/quiver/{id}
```

**Path Parameters**:
- `id` (string): Quiver ID

**Response**:
```json
{
  "id": "quiver-1",
  "name": "My Game Server",
  "description": "Main game server instance",
  "banner": "https://example.com/banner.png",
  "url": "https://example.com",
  "security": "trusted",
  "maintainers": ["admin@example.com"],
  "version": "1.0.0",
  "installed_arrows": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "namespace": "cs2",
      "name": "Counter-Strike 2 Server",
      "version": "1.0.0",
      "status": "running",
      "install_path": "./arrows/cs2",
      "pid": 12345,
      "uptime": "2h 30m"
    }
  ],
  "listed_arrows": [
    {
      "namespace": "minecraft",
      "name": "Minecraft Server",
      "description": "A Minecraft server",
      "version": "1.20.1"
    }
  ],
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T12:45:00Z"
}
```

### Update Quiver Repository

Update a Quiver repository.

```http
PUT /api/v1/quiver/{id}
```

**Path Parameters**:
- `id` (string): Quiver ID

**Request Body**:
```json
{
  "name": "Updated Game Server",
  "description": "Updated description",
  "banner": "https://example.com/new-banner.png"
}
```

**Response**:
```json
{
  "id": "quiver-1",
  "name": "Updated Game Server",
  "description": "Updated description",
  "banner": "https://example.com/new-banner.png",
  "url": "https://example.com",
  "security": "trusted",
  "maintainers": ["admin@example.com"],
  "version": "1.0.0",
  "updated_at": "2024-01-15T13:00:00Z"
}
```

### Delete Quiver Repository

Delete a Quiver repository.

```http
DELETE /api/v1/quiver/{id}
```

**Path Parameters**:
- `id` (string): Quiver ID

**Response**:
```json
{
  "message": "Quiver deleted successfully",
  "id": "quiver-1"
}
```

## System Operations

### Get System Status

Get system status and information.

```http
GET /api/v1/system/status
```

**Response**:
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "2h 30m",
  "memory_usage": "512MB",
  "cpu_usage": "15%",
  "disk_usage": "2.5GB",
  "network_status": "connected",
  "active_arrows": 1,
  "total_quivers": 1
}
```

### Get System Information

Get detailed system information.

```http
GET /api/v1/system/info
```

**Response**:
```json
{
  "system": {
    "os": "linux",
    "arch": "amd64",
    "kernel": "5.4.0-74-generic",
    "hostname": "quiver-server"
  },
  "runtime": {
    "go_version": "1.24.2",
    "quiver_version": "1.0.0",
    "build_time": "2024-01-15T10:00:00Z",
    "git_commit": "abc123def456"
  },
  "configuration": {
    "api_host": "0.0.0.0",
    "api_port": 40257,
    "log_level": "info",
    "database_path": "./.db"
  }
}
```

## Error Handling

### Error Response Format

All errors follow a consistent format:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid request parameters",
    "details": {
      "field": "name",
      "reason": "Name is required"
    },
    "timestamp": "2024-01-15T10:30:00Z",
    "request_id": "req-123456"
  }
}
```

### Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `VALIDATION_ERROR` | 400 | Invalid request parameters |
| `NOT_FOUND` | 404 | Resource not found |
| `CONFLICT` | 409 | Resource conflict |
| `UNAUTHORIZED` | 401 | Authentication required |
| `FORBIDDEN` | 403 | Access denied |
| `INTERNAL_ERROR` | 500 | Internal server error |

### Error Examples

**Validation Error**:
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid request parameters",
    "details": {
      "field": "variables.MAX_PLAYERS",
      "reason": "Value must be between 2 and 64"
    }
  }
}
```

**Not Found Error**:
```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "Arrow not found",
    "details": {
      "namespace": "nonexistent",
      "reason": "No arrow found with namespace 'nonexistent'"
    }
  }
}
```

## Rate Limiting

Currently, there are no rate limits implemented. Future versions may include:

- **Per-IP Limits**: 100 requests per minute
- **Per-User Limits**: 1000 requests per hour
- **Burst Limits**: 10 requests per second

## Authentication

Currently, the API does not require authentication. Future versions will include:

- **JWT Tokens**: Bearer token authentication
- **API Keys**: Key-based authentication
- **OAuth2**: Third-party authentication

## Repository Specification

### Repository Syntax

Quiver supports specifying repositories using the `repository@package` syntax:

- **Search**: `arrows/@cs2` - searches for "cs2" in repositories containing "arrows/"
- **Install**: `github.com/repo@cs2` - installs "cs2" from "github.com/repo"
- **Update**: `arrows/@cs2` - updates "cs2" from repositories containing "arrows/"

### Repository Sources

1. **Local Repositories**: `./pkgs` (local file system)
2. **Remote Repositories**: `https://raw.githubusercontent.com/rabbytesoftware/quiver.arrows/main`
3. **Custom Repositories**: Any HTTP/HTTPS URL serving Arrow packages

## SDK Examples

### JavaScript/Node.js

```javascript
const axios = require('axios');

const api = axios.create({
  baseURL: 'http://localhost:40257/api/v1',
  headers: {
    'Content-Type': 'application/json'
  }
});

// Search arrows
async function searchArrows(query) {
  const response = await api.get(`/arrow/search?q=${query}`);
  return response.data;
}

// Install arrow
async function installArrow(namespace, variables = {}) {
  const response = await api.post(`/arrow/${namespace}/install`, {
    variables
  });
  return response.data;
}
```

### Python

```python
import requests

class QuiverAPI:
    def __init__(self, base_url='http://localhost:40257/api/v1'):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({
            'Content-Type': 'application/json'
        })
    
    def search_arrows(self, query):
        response = self.session.get(f'{self.base_url}/arrow/search', params={'q': query})
        return response.json()
    
    def install_arrow(self, namespace, variables=None):
        data = {'variables': variables or {}}
        response = self.session.post(f'{self.base_url}/arrow/{namespace}/install', json=data)
        return response.json()
```

### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

type QuiverAPI struct {
    BaseURL string
    Client  *http.Client
}

func NewQuiverAPI(baseURL string) *QuiverAPI {
    return &QuiverAPI{
        BaseURL: baseURL,
        Client:  &http.Client{},
    }
}

func (api *QuiverAPI) SearchArrows(query string) (map[string]interface{}, error) {
    resp, err := api.Client.Get(api.BaseURL + "/arrow/search?q=" + query)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    return result, nil
}

func (api *QuiverAPI) InstallArrow(namespace string, variables map[string]string) (map[string]interface{}, error) {
    data := map[string]interface{}{
        "variables": variables,
    }
    
    jsonData, _ := json.Marshal(data)
    resp, err := api.Client.Post(api.BaseURL+"/arrow/"+namespace+"/install", "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    return result, nil
}
```

---

*For more information about the system architecture, see the [Architecture Overview](architecture-overview.md) and [Domain Models](domain-models.md) documentation.*
