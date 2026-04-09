package bootstrap

type store interface {
	ApplySchema() error
	SeedAdmin()
	SeedPartners()
	SeedInitialCatalog()
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
	s.store.SeedAdmin()
	s.store.SeedPartners()
	s.store.SeedInitialCatalog()
	return nil
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
