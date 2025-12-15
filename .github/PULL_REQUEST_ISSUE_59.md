## Description
Implements an optional in-memory caching layer for the Core database module using the builder pattern. The cache wraps the base database interface with a cache layer (decorator pattern) to improve read performance and reduce database I/O. This is step 1 of the broader persistence initiative.

## Type of Change
- [ ] 🐛 Bug fix (fix/*)
- [x] ✨ New feature (feature/*)
- [ ] 🔧 Enhancement (enhancement/*)
- [ ] 🚑 Hotfix (hotfix/*)
- [ ] 🚀 Release (release/*)

## Related Issues
Closes #59

## Changes Made
- Added `Cache` interface with `Get`, `Set`, `Delete`, and `Flush` methods supporting TTL
- Implemented `GoCache` using `github.com/patrickmn/go-cache` library (as specified in issue #59)
- Created `RepositoryCache` decorator that wraps base repository with transparent caching
- Implemented `DatabaseBuilder` pattern with optional `WithCache()` method for flexible composition
- Added cache configuration to config system (`cache.enabled`, `cache.default_ttl`, `cache.cleanup_interval`)
- Added cache settings to `default.yaml` (disabled by default)
- Created helper function to convert YAML config to `CacheConfig`
- Maintained backwards compatibility: `NewDatabase()` function still works without changes
- Comprehensive test suite with ~34 tests covering all components
- Added `github.com/patrickmn/go-cache` dependency

### New Files
- `internal/core/database/cache/interface.go` - Cache interface definition
- `internal/core/database/cache/config.go` - Cache configuration struct and defaults
- `internal/core/database/cache/memory_cache.go` - Alternative in-memory cache implementation (fallback)
- `internal/core/database/cache/gocache.go` - go-cache implementation using `github.com/patrickmn/go-cache`
- `internal/core/database/cache/repository_cache.go` - Repository decorator with caching
- `internal/core/database/cache/config_helper.go` - YAML config to CacheConfig converter
- `internal/core/database/builder.go` - DatabaseBuilder pattern implementation
- `internal/core/database/cache/cache_test.go` - Cache interface tests
- `internal/core/database/cache/repository_cache_test.go` - Repository cache tests
- `internal/core/database/builder_test.go` - Builder pattern tests
- `internal/core/database/integration_test.go` - Integration tests

### Modified Files
- `internal/core/config/config.go` - Added Cache struct and GetCache() function
- `internal/core/config/default.yaml` - Added cache configuration section

## Breaking Changes
- [x] This PR introduces breaking changes
- [x] Migration guide provided (if yes)

### Migration Guide
No migration required. The cache is **optional and disabled by default**. Existing code using `NewDatabase()` continues to work without modification.

To enable caching, use the new builder pattern:
```go
// With cache enabled
db, err := database.NewDatabaseBuilder[MyEntity](ctx, "mydb").
    WithCache(cache.CacheConfig{
        Enabled:         true,
        DefaultTTL:      5 * time.Minute,
        CleanupInterval: 1 * time.Minute,
    }).
    Build()

// Without cache (backwards compatible)
db, err := database.NewDatabase[MyEntity](ctx, "mydb")
```

Or enable via configuration file:
```yaml
config:
  cache:
    enabled: true
    default_ttl: "5m"
    cleanup_interval: "1m"
```

## Checklist
- [x] Code follows Clean Architecture principles
- [x] Code follows project style guidelines
- [x] Self-review completed
- [x] Comments added for complex logic
- [ ] All documentation updated in `./docs/` folder
- [ ] Tests pass locally (`make test`)
- [ ] Coverage requirements met (`make test-coverage`)
- [ ] No linting errors (`make lint`)
- [ ] Security scan passes (`make security`)
- [x] Branch name follows convention (enhancement/*, feature/*, fix/*, hotfix/*, release/*)

## Screenshots/Logs (if applicable)

### Test Coverage Summary
| Component | Tests | Coverage |
|-----------|-------|----------|
| Cache Interface | 11 | Hit/miss, TTL, flush, concurrency |
| Repository Cache | 13 | Invalidation, decorator transparency |
| Builder Pattern | 7 | Build with/without cache |
| Integration | 3 | End-to-end CRUD with caching |

### Key Features
- **Optional**: Cache is disabled by default, zero overhead when not used
- **Thread-safe**: Uses `go-cache` library which is thread-safe by design
- **Automatic invalidation**: Cache cleared on Create/Update/Delete
- **TTL support**: Entries expire after configured duration
- **go-cache library**: Uses `github.com/patrickmn/go-cache` as specified in issue #59

