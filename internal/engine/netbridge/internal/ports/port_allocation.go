package ports

import "github.com/rabbytesoftware/quiver/internal/domain/netbridge"

// PortAllocation represents a single allocated port and its metadata.
type PortAllocation struct {
	Port      int                `db:"port"      json:"port"`
	Protocol  netbridge.Protocol `db:"protocol"  json:"protocol"`
	OwnerKey  string             `db:"owner_key" json:"owner_key"`
	Forwarded bool               `db:"forwarded" json:"forwarded"`
}
