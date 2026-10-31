-- Keep public service pages in title order without sorting every published row.
CREATE INDEX service_listings_published_title_page_idx
    ON service_listings (lower(title), id)
    WHERE state = 'published';
