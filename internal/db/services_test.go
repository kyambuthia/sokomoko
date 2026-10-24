package db

import (
	"errors"
	"sync"
	"testing"
)

func serviceFixture(slug string) ServiceListing {
	return ServiceListing{Slug: slug, Title: "Demo haircut", Description: "Sample service description only", Category: "hair", DurationMinutes: 30, PriceMode: "fixed", PriceCents: 100000, Currency: "KES", PublicArea: "Westlands", DeliveryMode: "premises", State: "draft"}
}
func TestServiceListingsOwnershipVisibilityAndVersioning(t *testing.T) {
	s := newTestStore(t)
	partner, owner, _ := chatShop(t, s)
	_, outsider, _ := chatShop(t, s)
	v, err := s.SaveService(t.Context(), owner, serviceFixture("demo-haircut"))
	chatMust(t, err)
	_, err = s.PublicService(t.Context(), v.Slug)
	chatWantError(t, err, ErrServiceNotFound)
	items, total, err := s.OwnServices(t.Context(), owner, 20, 0)
	chatMust(t, err)
	if len(items) != 1 || total != 1 {
		t.Fatalf("own=%+v,total=%d", items, total)
	}
	items, total, err = s.OwnServices(t.Context(), outsider, 20, 0)
	chatMust(t, err)
	if len(items) != 0 || total != 0 {
		t.Fatal("cross-shop draft leak")
	}
	v.State = "published"
	_, err = s.SaveService(t.Context(), outsider, *v)
	chatWantError(t, err, ErrServiceNotFound)
	published, err := s.SaveService(t.Context(), owner, *v)
	chatMust(t, err)
	_, err = s.SaveService(t.Context(), owner, *v)
	chatWantError(t, err, ErrServiceConflict)
	got, err := s.PublicService(t.Context(), v.Slug)
	chatMust(t, err)
	if got.Version != 2 || got.State != "published" {
		t.Fatalf("public=%+v", got)
	}
	items, total, err = s.BrowseServices(t.Context(), ServiceFilter{Query: "%", Limit: 20})
	chatMust(t, err)
	if total != 0 || len(items) != 0 {
		t.Fatal("search wildcard was interpreted")
	}
	items, total, err = s.BrowseServices(t.Context(), ServiceFilter{Query: "HAIRCUT", Category: "hair", Area: "west", Limit: 20})
	chatMust(t, err)
	if total != 1 || len(items) != 1 {
		t.Fatal("published search failed")
	}
	published.State = "archived"
	archived, err := s.SaveService(t.Context(), owner, *published)
	chatMust(t, err)
	_, err = s.PublicService(t.Context(), v.Slug)
	chatWantError(t, err, ErrServiceNotFound)
	archived.State = "published"
	_, err = s.SaveService(t.Context(), owner, *archived)
	chatMust(t, err)
	_, err = s.DB.Exec(`UPDATE partners SET status='suspended' WHERE id=$1`, partner)
	chatMust(t, err)
	_, err = s.PublicService(t.Context(), v.Slug)
	chatWantError(t, err, ErrServiceNotFound)
	_, total, err = s.BrowseServices(t.Context(), ServiceFilter{Limit: 20})
	chatMust(t, err)
	if total != 0 {
		t.Fatal("suspended service remained public")
	}
	_, err = s.SaveService(t.Context(), owner, *archived)
	chatWantError(t, err, ErrServiceNotFound)
	_, err = s.DB.Exec(`UPDATE partners SET status='active' WHERE id=$1`, partner)
	chatMust(t, err)
	_, err = s.DB.Exec(`DELETE FROM partner_users WHERE user_id=$1`, owner)
	chatMust(t, err)
	_, err = s.SaveService(t.Context(), owner, serviceFixture("removed-member"))
	chatWantError(t, err, ErrServiceNotFound)
}
func TestServiceConcurrentEditsDoNotOverwrite(t *testing.T) {
	s := newTestStore(t)
	_, owner, _ := chatShop(t, s)
	v, err := s.SaveService(t.Context(), owner, serviceFixture("concurrent-service"))
	chatMust(t, err)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	start := make(chan struct{})
	for _, title := range []string{"First edit", "Second edit"} {
		wg.Go(func() {
			<-start
			candidate := *v
			candidate.Title = title
			_, err := s.SaveService(t.Context(), owner, candidate)
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrServiceConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("success=%d conflict=%d", successes, conflicts)
	}
	var count int
	chatMust(t, s.DB.QueryRow(`SELECT count(*) FROM audit_logs WHERE target_type='service' AND target_id=$1`, v.ID).Scan(&count))
	if count != 2 {
		t.Fatalf("audit count=%d", count)
	}
}
