package ports

// PortAllocation represents a single allocated port and its metadata.
type PortAllocation struct {
	Port      int
	Protocol  Protocol
	OwnerKey  string
	Forwarded bool
}
