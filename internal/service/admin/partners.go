package admin

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/kyambuthia/sokomoko/internal/auth"
	"github.com/kyambuthia/sokomoko/internal/db"
	"github.com/kyambuthia/sokomoko/internal/slug"
)

var (
	ErrForbiddenPartnerAction = errors.New("forbidden partner action")
	ErrInvalidPartner         = errors.New("invalid partner")
	ErrDuplicatePartner       = errors.New("duplicate partner")
	ErrPartnerNotFound        = errors.New("partner not found")
	ErrDuplicateAccount       = errors.New("duplicate account")
)

// AccountInputError carries a user-facing reason a new account was rejected.
type AccountInputError struct{ Reason string }

func (e *AccountInputError) Error() string { return e.Reason }

const maxPartnerName = 120

type Partner struct {
	ID           int
	Name         string
	Slug         string
	ContactEmail string
	Status       string
}

type PartnerAccount struct {
	UserID      int
	Username    string
	Email       string
	PartnerID   int
	PartnerName string
}

type PartnerInput struct {
	Name         string
	Slug         string
	ContactEmail string
}

type PartnerAccountInput struct {
	PartnerID string
	Username  string
	Email     string
	Password  string
}

// canManagePartners reports whether actor may onboard and suspend partners.
// Vendor onboarding grants access to fulfillment data, so it is admin only.
func canManagePartners(actor *Actor) bool {
	return actor != nil && actor.Role == db.RoleAdmin
}

// Partners lists every partner and every active partner login.
func (s *Service) Partners() ([]Partner, []PartnerAccount, error) {
	partners, err := s.store.ListPartners()
	if err != nil {
		return nil, nil, err
	}
	accounts, err := s.store.ListPartnerAccounts()
	if err != nil {
		return nil, nil, err
	}
	mappedPartners := make([]Partner, 0, len(partners))
	for _, p := range partners {
		mappedPartners = append(mappedPartners, Partner{ID: p.ID, Name: p.Name, Slug: p.Slug, ContactEmail: p.ContactEmail, Status: p.Status})
	}
	mappedAccounts := make([]PartnerAccount, 0, len(accounts))
	for _, a := range accounts {
		mappedAccounts = append(mappedAccounts, PartnerAccount(a))
	}
	return mappedPartners, mappedAccounts, nil
}

// CreatePartner onboards a vendor. An empty slug is derived from the name.
func (s *Service) CreatePartner(actor *Actor, input PartnerInput) error {
	if !canManagePartners(actor) {
		return ErrForbiddenPartnerAction
	}
	name := strings.TrimSpace(input.Name)
	email := strings.ToLower(strings.TrimSpace(input.ContactEmail))
	partnerSlug := strings.ToLower(strings.TrimSpace(input.Slug))
	if partnerSlug == "" {
		partnerSlug = slug.Make(name)
	}
	if name == "" || len([]rune(name)) > maxPartnerName || !slug.Valid(partnerSlug) {
		return ErrInvalidPartner
	}
	if email != "" {
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
			return ErrInvalidPartner
		}
	}
	id, err := s.store.CreatePartner(db.Partner{Name: name, Slug: partnerSlug, ContactEmail: email})
	if db.IsUniqueConstraintError(err) {
		return ErrDuplicatePartner
	}
	if err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(actor.ID, "partner.create", "partner", int(id), "slug="+partnerSlug)
	return nil
}

// SetPartnerStatus suspends or reactivates a partner.
func (s *Service) SetPartnerStatus(actor *Actor, idRaw, status string) error {
	if !canManagePartners(actor) {
		return ErrForbiddenPartnerAction
	}
	if status != "active" && status != "suspended" {
		return ErrInvalidPartner
	}
	id, err := parsePositiveInt(idRaw)
	if err != nil {
		return ErrPartnerNotFound
	}
	found, err := s.store.SetPartnerStatus(id, status)
	if err != nil {
		return err
	}
	if !found {
		return ErrPartnerNotFound
	}
	_ = s.store.CreateAuditLog(actor.ID, "partner.status", "partner", id, "status="+status)
	return nil
}

// UpdatePartner renames a partner and changes its contact email.
func (s *Service) UpdatePartner(actor *Actor, idRaw string, input PartnerInput) error {
	if !canManagePartners(actor) {
		return ErrForbiddenPartnerAction
	}
	id, err := parsePositiveInt(idRaw)
	if err != nil {
		return ErrPartnerNotFound
	}
	name := strings.TrimSpace(input.Name)
	email := strings.ToLower(strings.TrimSpace(input.ContactEmail))
	if name == "" || len([]rune(name)) > maxPartnerName {
		return ErrInvalidPartner
	}
	if email != "" {
		if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
			return ErrInvalidPartner
		}
	}
	found, err := s.store.UpdatePartnerDetails(id, name, email)
	if err != nil {
		return err
	}
	if !found {
		return ErrPartnerNotFound
	}
	_ = s.store.CreateAuditLog(actor.ID, "partner.update", "partner", id, "name="+name)
	return nil
}

// CreatePartnerAccount creates a partner-role login tied to one partner.
func (s *Service) CreatePartnerAccount(actor *Actor, input PartnerAccountInput) error {
	if !canManagePartners(actor) {
		return ErrForbiddenPartnerAction
	}
	partnerID, err := parsePositiveInt(input.PartnerID)
	if err != nil {
		return ErrPartnerNotFound
	}
	partner, err := s.store.GetPartnerByID(partnerID)
	if err != nil {
		return err
	}
	if partner == nil {
		return ErrPartnerNotFound
	}
	username := strings.TrimSpace(input.Username)
	email := strings.ToLower(strings.TrimSpace(input.Email))
	if problem := auth.ValidateNewAccount(username, email, input.Password); problem != "" {
		return &AccountInputError{Reason: problem}
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		return err
	}
	userID, err := s.store.CreatePartnerUser(db.User{Username: username, Email: email, PasswordHash: hash}, partnerID)
	if db.IsUniqueConstraintError(err) {
		return ErrDuplicateAccount
	}
	if err != nil {
		return err
	}
	_ = s.store.CreateAuditLog(actor.ID, "partner.account.create", "user", int(userID), "partner="+partner.Slug)
	return nil
}
