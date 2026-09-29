-- +goose Up

-- Slices of the marketplace, the factory floor, the review feed and the
-- giveaway tool, so the newer tools have real rows to read. In production
-- each is its own service behind the same tool ports (tools.Marketplace,
-- tools.Factory, tools.Reviews and tools.Giveaways).

CREATE TABLE marketplace_stores (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    owner_email text NOT NULL,
    owner_name  text NOT NULL,
    description text NOT NULL DEFAULT '',
    has_banner  boolean NOT NULL DEFAULT false,
    views_30d   integer NOT NULL DEFAULT 0,
    opened_at   timestamptz NOT NULL
);

CREATE TABLE marketplace_listings (
    id          text PRIMARY KEY,
    store_id    text NOT NULL REFERENCES marketplace_stores (id),
    title       text NOT NULL,
    description text NOT NULL DEFAULT '',
    tags        text[] NOT NULL DEFAULT '{}',
    product     text NOT NULL,
    price_cents integer NOT NULL CHECK (price_cents > 0),
    status      text NOT NULL,
    units_sold  integer NOT NULL DEFAULT 0
);
CREATE INDEX marketplace_listings_store_idx ON marketplace_listings (store_id);

-- Brands and characters sellers may not use without a license.
CREATE TABLE protected_marks (
    term  text PRIMARY KEY,
    owner text NOT NULL
);

CREATE TABLE production_stations (
    id            text PRIMARY KEY,
    products      text[] NOT NULL,
    units_per_day integer NOT NULL CHECK (units_per_day > 0)
);

CREATE TABLE production_jobs (
    id         text PRIMARY KEY,
    order_id   text NOT NULL,
    product    text NOT NULL,
    quantity   integer NOT NULL CHECK (quantity > 0),
    station_id text NOT NULL REFERENCES production_stations (id),
    status     text NOT NULL,
    queued_at  timestamptz NOT NULL,
    ship_by    date NOT NULL
);
CREATE INDEX production_jobs_open_idx ON production_jobs (queued_at) WHERE status IN ('QUEUED', 'PRINTING');

CREATE TABLE storefront_reviews (
    id         text PRIMARY KEY,
    order_id   text NOT NULL,
    product    text NOT NULL,
    rating     integer NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body       text NOT NULL,
    created_at timestamptz NOT NULL
);
CREATE INDEX storefront_reviews_recent_idx ON storefront_reviews (created_at DESC);

CREATE TABLE giveaway_entries (
    giveaway_id text NOT NULL,
    email       text NOT NULL,
    name        text NOT NULL,
    address     text NOT NULL,
    entered_at  timestamptz NOT NULL,
    PRIMARY KEY (giveaway_id, email)
);

-- +goose Down
DROP TABLE giveaway_entries;
DROP TABLE storefront_reviews;
DROP TABLE production_jobs;
DROP TABLE production_stations;
DROP TABLE protected_marks;
DROP TABLE marketplace_listings;
DROP TABLE marketplace_stores;
