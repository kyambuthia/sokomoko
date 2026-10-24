package db

import (
	"context"
	"fmt"
)

// SeedDemoServices adds illustrative directory entries without changing any
// existing seller edits or publication state. Only the development seed workflow
// calls this method; migrations never create public sample content.
func (s *Store) SeedDemoServices(ctx context.Context) error {
	partner, err := s.GetPartnerBySlug(ctx, "urban_goods")
	if err != nil {
		return err
	}
	if partner == nil {
		return fmt.Errorf("seed demo services: demo partner is missing")
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO service_listings
 (partner_id,slug,title,description,category,duration_minutes,price_mode,price_cents,currency,public_area,delivery_mode,state)
 VALUES
 ($1,'demo-neighbourhood-haircut','Demo neighbourhood haircut','Sample haircut offering for exploring the directory. This is not a real salon and no appointments are accepted.','hair',30,'fixed',50000,'KES','Westlands, Nairobi','premises','published'),
 ($1,'demo-home-care-visit','Demo home care visit','Sample home maintenance offering for exploring mobile services. No actual provider will visit and no appointments are accepted.','home',60,'from',150000,'KES','Kilimani, Nairobi','mobile','published'),
 ($1,'demo-remote-consultation','Demo remote design consultation','Sample remote design offering with quote-based pricing. No consultations or payments are accepted.','other',45,'quote',0,'KES','Nairobi','remote','published')
 ON CONFLICT (slug) DO NOTHING`, partner.ID)
	if err != nil {
		return fmt.Errorf("seed demo services: %w", err)
	}
	return nil
}
