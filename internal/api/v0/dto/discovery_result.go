package dto

import "fmt"

// DiscoveryResult wraps a list of discovery query results.
type DiscoveryResult struct {
	// Items are the arrows and/or collections matching the query.
	Items []DiscoveryItem `json:"items" yaml:"items"`
	// Total is the count of items in the result.
	Total int `json:"total" yaml:"total"`
	// Query is the pattern used (empty = all items).
	Query string `json:"query" yaml:"query"`
}

// DiscoveryItem is a union of Arrow and Collection, tagged by Kind.
type DiscoveryItem struct {
	// Kind is "arrow" or "collection".
	Kind string `json:"kind" yaml:"kind"`
	// Arrow is the arrow data (present if Kind == "arrow").
	Arrow *ArrowListItemDTO `json:"arrow,omitempty" yaml:"arrow,omitempty"`
	// Collection is the collection data (present if Kind == "collection").
	Collection *CollectionListItemDTO `json:"collection,omitempty" yaml:"collection,omitempty"`
}

// CheckPayload validates the DiscoveryResult structure.
func (d *DiscoveryResult) CheckPayload() error {
	if d.Items == nil {
		return fmt.Errorf("discovery result: items must not be nil")
	}
	if d.Total < 0 {
		return fmt.Errorf("discovery result: total must be non-negative")
	}
	return nil
}
