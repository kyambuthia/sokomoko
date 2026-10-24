package db

import (
	"errors"
	"fmt"
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

func TestServiceDiscoveryFiltersLocationTypeWithoutLeakingDrafts(t *testing.T) {
	s := newTestStore(t)
	_, owner, _ := chatShop(t, s)
	for _, mode := range []string{"premises", "mobile", "remote"} {
		for _, state := range []string{"published", "draft", "archived"} {
			v := serviceFixture("filter-" + mode + "-" + state)
			v.DeliveryMode, v.State = mode, state
			_, err := s.SaveService(t.Context(), owner, v)
			chatMust(t, err)
		}
	}
	for _, mode := range []string{"premises", "mobile", "remote"} {
		items, total, err := s.BrowseServices(t.Context(), ServiceFilter{DeliveryMode: mode, Area: "west", Category: "hair", Limit: 1})
		chatMust(t, err)
		if total != 1 || len(items) != 1 || items[0].DeliveryMode != mode || items[0].State != "published" {
			t.Fatalf("mode=%s total=%d items=%+v", mode, total, items)
		}
	}
	items, total, err := s.BrowseServices(t.Context(), ServiceFilter{Limit: 2, Offset: 2})
	chatMust(t, err)
	if total != 3 || len(items) != 1 {
		t.Fatalf("unfiltered pagination total=%d items=%+v", total, items)
	}
}

func TestServiceDiscoveryScopesBusinessAndVisibility(t *testing.T) {
	s := newTestStore(t)
	partner, owner, _ := chatShop(t, s)
	_, other, _ := chatShop(t, s)
	for _, actor := range []int{owner, other} {
		for _, state := range []string{"published", "draft", "archived"} {
			v := serviceFixture(fmt.Sprintf("business-%d-%s", actor, state))
			v.State = state
			_, err := s.SaveService(t.Context(), actor, v)
			chatMust(t, err)
		}
	}
	filter := ServiceFilter{PartnerID: partner, Area: "west", Category: "hair", DeliveryMode: "premises", Limit: 1}
	items, total, err := s.BrowseServices(t.Context(), filter)
	chatMust(t, err)
	if total != 1 || len(items) != 1 || items[0].PartnerID != partner || items[0].State != "published" {
		t.Fatalf("total=%d items=%+v", total, items)
	}
	filter.Offset = 1
	items, total, err = s.BrowseServices(t.Context(), filter)
	chatMust(t, err)
	if total != 1 || len(items) != 0 {
		t.Fatal("business pagination count mismatch")
	}
	filter.Offset = 0
	filter.Area = "nonexistent"
	_, total, err = s.BrowseServices(t.Context(), filter)
	chatMust(t, err)
	if total != 0 {
		t.Fatal("business filter ignored area")
	}
	filter.Area = ""
	_, err = s.DB.Exec(`UPDATE partners SET status='suspended' WHERE id=$1`, partner)
	chatMust(t, err)
	_, total, err = s.BrowseServices(t.Context(), filter)
	chatMust(t, err)
	if total != 0 {
		t.Fatal("business filter exposed suspended shop")
	}
	filter.PartnerID = 2147483647
	_, total, err = s.BrowseServices(t.Context(), filter)
	chatMust(t, err)
	if total != 0 {
		t.Fatal("unknown business returned listings")
	}
}

func TestOwnServicesStateFilterScopesCountsAndPagination(t *testing.T) {
	s := newTestStore(t)
	partner, owner, _ := chatShop(t, s)
	_, outsider, _ := chatShop(t, s)
	for i, state := range []string{"draft", "draft", "published", "archived"} {
		listing := serviceFixture(fmt.Sprintf("state-filter-%d", i))
		listing.State = state
		_, err := s.SaveService(t.Context(), owner, listing)
		chatMust(t, err)
	}
	for _, state := range []string{"", "draft", "published", "archived"} {
		want := 1
		if state == "" {
			want = 4
		} else if state == "draft" {
			want = 2
		}
		items, total, err := s.OwnServicesByState(t.Context(), owner, state, 1, 0)
		chatMust(t, err)
		if len(items) != 1 || total != want || (state != "" && items[0].State != state) {
			t.Fatalf("state=%q items=%+v total=%d", state, items, total)
		}
		next, nextTotal, err := s.OwnServicesByState(t.Context(), owner, state, 1, 1)
		chatMust(t, err)
		if nextTotal != want || (want == 1 && len(next) != 0) || (want > 1 && (len(next) != 1 || next[0].ID == items[0].ID || (state != "" && next[0].State != state))) {
			t.Fatalf("state=%q second page=%+v total=%d", state, next, nextTotal)
		}
		items, total, err = s.OwnServicesByState(t.Context(), outsider, state, 10, 0)
		chatMust(t, err)
		if len(items) != 0 || total != 0 {
			t.Fatalf("state=%q leaked another shop's services", state)
		}
	}
	_, _, err := s.OwnServicesByState(t.Context(), owner, "unknown", 10, 0)
	chatWantError(t, err, ErrServiceNotFound)
	_, err = s.DB.ExecContext(t.Context(), `UPDATE partners SET status='suspended' WHERE id=$1`, partner)
	chatMust(t, err)
	items, total, err := s.OwnServicesByState(t.Context(), owner, "draft", 10, 0)
	chatMust(t, err)
	if len(items) != 0 || total != 0 {
		t.Fatal("suspended shop retained listing access")
	}
}
