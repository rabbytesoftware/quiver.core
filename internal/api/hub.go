package api

import apphub "github.com/rabbytesoftware/quiver.core/internal/app/hub"

// WSVersion is the interface each API version's WS handler must implement.
// Aliased from apphub.Subscriber so both packages refer to the same type.
type WSVersion = apphub.Subscriber

// Hub fans out domain broadcasts to all registered API version WS handlers.
// Aliased from apphub.Hub so it can be owned by the app layer and passed here.
type Hub = apphub.Hub

func NewHub() *Hub { return apphub.NewHub() }
