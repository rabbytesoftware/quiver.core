package ports

import "github.com/rabbytesoftware/quiver/internal/domain/netbridge"

type PortAllocation struct {
	Port      int `gorm:"primaryKey"`
	Protocol  netbridge.Protocol
	OwnerKey  string
	Forwarded bool
}

func (PortAllocation) TableName() string { return "port_allocations" }
