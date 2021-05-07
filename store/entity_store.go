package store

// EntityStore is the general interface for storing different entity types.
type EntityStore interface {
	// Add adds an entity to the store.
	// It overwrites the old entity if one with the same id was already stored.
	// Returns an error if the entity is of invalid type for the concrete implementation.
	Add(entity Entity)

	// Get returns a entity from the store.
	// If the entity does not exist in the store, ok will be false.
	Get(id string) (entity Entity, ok bool)

	// Delete deletes the entity with the passed id from the store.
	Delete(id string)
}

type Entity interface {
	// Id returns the id of the given entity.
	Id() string
}
