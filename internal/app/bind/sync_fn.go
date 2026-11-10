package bind

import "github.com/rabbytesoftware/quiver.core/internal/app/bind/internal/binder"

// SyncFn is the body of a sync projection: it runs inside the transaction that
// stores the event, so everything it writes through tx commits or rolls back
// with that event. See the asynx Builder's WithSyncProjection.
type SyncFn[T any] = binder.SyncFn[T]
