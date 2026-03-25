package bootstrap

import "github.com/kyambuthia/sokomoko/internal/db"

// Initialize runs startup seed workflows for a fresh deployment.
// It is safe to call multiple times; seed operations are idempotent.
func Initialize(store *db.Store) error {
	if store == nil {
		return nil
	}
	store.SeedAdmin()
	store.SeedInitialCatalog()
	return nil
}
