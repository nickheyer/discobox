package types

// StorageEvent represents a configuration change
type StorageEvent struct {
	Type   string      // created, updated, deleted
	Kind   string      // service, route
	ID     string
	Object interface{}
}