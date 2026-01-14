package error

import "errors"

var (
	ErrInvalidCacheConfig = errors.New("INVALID_CACHE_CONFIGURATION")
	ErrIDExtractionFailed = errors.New("ID_EXTRACTION_FAILED")
	ErrMissingCache       = errors.New("MISSING_CACHE")
	ErrMissingBase        = errors.New("MISSING_BASE")
	ErrInvalidCacheValue  = errors.New("INVALID_CACHE_VALUE")
	ErrCacheWriteFailed   = errors.New("CACHE_WRITE_FAILED")
	ErrEntityNotFound     = errors.New("ENTITY_NOT_FOUND")
)
