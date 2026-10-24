package bootstrap

import "context"

type store interface {
	ApplySchema() error
	SeedAdmin(ctx context.Context)
	SeedPartners(ctx context.Context)
	SeedInitialCatalog(ctx context.Context)
	SeedDemoAccounts(ctx context.Context) error
}

type Service struct {
	store store
}

func New(store store) *Service {
	return &Service{store: store}
}

func (s *Service) Migrate() error {
	if s == nil || s.store == nil {
		return nil
	}
	return s.store.ApplySchema()
}

func (s *Service) Seed() error {
	if s == nil || s.store == nil {
		return nil
	}
	s.store.SeedAdmin(context.Background())
	s.store.SeedPartners(context.Background())
	s.store.SeedInitialCatalog(context.Background())
	return s.store.SeedDemoAccounts(context.Background())
}

func (s *Service) Prepare(seed bool) error {
	if err := s.Migrate(); err != nil {
		return err
	}
	if seed {
		return s.Seed()
	}
	return nil
}

// Initialize runs startup seed workflows for a fresh deployment.
// It is safe to call multiple times; seed operations are idempotent.
func Initialize(store store) error {
	return New(store).Seed()
}
