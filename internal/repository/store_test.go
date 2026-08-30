package repository

// store is a test-only convenience helper for storing objects in unit tests.
func (r *Repository) store(objectType string, value any) ObjectID {
	id, err := r.storeObject(objectType, value)
	if err != nil {
		panic(err)
	}
	return id
}
