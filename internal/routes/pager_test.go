package routes

import (
	"net/http/httptest"
	"testing"
)

func TestPageParam(t *testing.T) {
	for raw, want := range map[string]int{"": 1, "3": 3, "0": 1, "-2": 1, "abc": 1, "99999999": maxPage} {
		r := httptest.NewRequest("GET", "/team?page="+raw, nil)
		if got := pageParam(r); got != want {
			t.Errorf("page=%q: got %d, want %d", raw, got, want)
		}
	}
}

func TestNewPagerKeepsQueryAndDropsPageOne(t *testing.T) {
	r := httptest.NewRequest("GET", "/team?q=ann&page=2", nil)
	p := newPager(r, 2, 50, 120)
	if p.Pages != 3 || p.PrevHref != "/team?q=ann" || p.NextHref != "/team?page=3&q=ann" {
		t.Fatalf("pager = %+v", p)
	}

	last := newPager(httptest.NewRequest("GET", "/account?page=3", nil), 3, 50, 120)
	if last.NextHref != "" || last.PrevHref != "/account?page=2" {
		t.Fatalf("last page = %+v", last)
	}
	if empty := newPager(httptest.NewRequest("GET", "/account", nil), 1, 20, 0); empty.Pages != 1 || empty.PrevHref != "" || empty.NextHref != "" {
		t.Fatalf("empty list = %+v", empty)
	}
}

func TestRedirectPastLastPage(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/team?q=ann&page=9", nil)
	if !redirectPastLastPage(w, r, 9, 50, 120) || w.Header().Get("Location") != "/team?page=3&q=ann" {
		t.Fatalf("redirect = %d %q", w.Code, w.Header().Get("Location"))
	}
	if redirectPastLastPage(httptest.NewRecorder(), r, 3, 50, 120) {
		t.Fatal("redirected from the last page")
	}
	if redirectPastLastPage(httptest.NewRecorder(), r, 9, 50, 0) {
		t.Fatal("redirected an empty list")
	}
}
