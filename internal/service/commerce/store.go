package commerce

import "github.com/kyambuthia/sokomoko/internal/db"

type store interface {
	AddToCart(userID, productID, quantity int) error
	GetCartItems(userID int) ([]CartItem, float64, error)
	GetProductByID(id int) (*productRecord, error)
	RemoveFromCart(userID, productID int) error
	UpdateCartQuantity(userID, productID, quantity int) error
}

type productRecord struct {
	ID            int
	StockQuantity int
}

type dbStore struct {
	store *db.Store
}

func newDBStore(store *db.Store) store {
	return &dbStore{store: store}
}

func (s *dbStore) AddToCart(userID, productID, quantity int) error {
	return s.store.AddToCart(userID, productID, quantity)
}

func (s *dbStore) GetCartItems(userID int) ([]CartItem, float64, error) {
	items, subtotal, err := s.store.GetCartItems(userID)
	if err != nil {
		return nil, 0, err
	}
	return mapCartItems(items), subtotal, nil
}

func (s *dbStore) GetProductByID(id int) (*productRecord, error) {
	product, err := s.store.GetProductByID(id)
	if err != nil || product == nil {
		return nil, err
	}
	return &productRecord{
		ID:            product.ID,
		StockQuantity: product.StockQuantity,
	}, nil
}

func (s *dbStore) RemoveFromCart(userID, productID int) error {
	return s.store.RemoveFromCart(userID, productID)
}

func (s *dbStore) UpdateCartQuantity(userID, productID, quantity int) error {
	return s.store.UpdateCartQuantity(userID, productID, quantity)
}
