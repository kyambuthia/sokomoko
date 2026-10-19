package routes

import (
	"net/http"
	"strconv"
	"strings"
)

// maxPage bounds ?page= so an absurd value cannot become a huge OFFSET.
const maxPage = 1_000_000

// Pager links the pages of a server-rendered list. The "pager" template
// partial renders it when there is more than one page.
type Pager struct {
	Page     int
	Pages    int
	PrevHref string
	NextHref string
}

// pageParam reads ?page=, defaulting to 1.
func pageParam(r *http.Request) int {
	page, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("page")))
	if err != nil || page < 1 {
		return 1
	}
	return min(page, maxPage)
}

func pageCount(limit, total int) int {
	return max(1, (total+limit-1)/limit)
}

// newPager builds links that keep the request's other query parameters.
func newPager(r *http.Request, page, limit, total int) Pager {
	p := Pager{Page: page, Pages: pageCount(limit, total)}
	if page > 1 {
		p.PrevHref = pageHref(r, min(page-1, p.Pages))
	}
	if page < p.Pages {
		p.NextHref = pageHref(r, page+1)
	}
	return p
}

// redirectPastLastPage sends a request for a page beyond the end of a
// non-empty list to its last page. It reports whether it redirected.
func redirectPastLastPage(w http.ResponseWriter, r *http.Request, page, limit, total int) bool {
	last := pageCount(limit, total)
	if total == 0 || page <= last {
		return false
	}
	http.Redirect(w, r, pageHref(r, last), http.StatusSeeOther)
	return true
}

func pageHref(r *http.Request, page int) string {
	query := r.URL.Query()
	if page <= 1 {
		query.Del("page")
	} else {
		query.Set("page", strconv.Itoa(page))
	}
	if encoded := query.Encode(); encoded != "" {
		return r.URL.Path + "?" + encoded
	}
	return r.URL.Path
}
