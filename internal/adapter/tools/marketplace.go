package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Store is a seller's shop on the marketplace.
type Store struct {
	ID          string    `json:"storeId"`
	Name        string    `json:"storeName"`
	OwnerEmail  string    `json:"customerEmail"` // sellers are customers too
	OwnerName   string    `json:"customerName"`
	Description string    `json:"description"`
	HasBanner   bool      `json:"hasBanner"`
	Views30d    int       `json:"views30d"`
	OpenedAt    time.Time `json:"openedAt"`
}

// Listing is one design for sale in a store.
type Listing struct {
	ID          string   `json:"listingId"`
	StoreID     string   `json:"storeId"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Product     string   `json:"product"`
	PriceCents  int      `json:"priceCents"`
	Status      string   `json:"listingStatus"` // IN_REVIEW, LIVE or HELD
	UnitsSold   int      `json:"unitsSold"`
}

// Mark is a registered brand, character or logo that sellers may not use
// without a license.
type Mark struct {
	Term  string `json:"term"`
	Owner string `json:"owner"`
}

// Marketplace is the store platform: sellers' shops, their listings and the
// register of protected marks that new designs are screened against. Tools
// depend on this port alone, not on the rest of the storefront (Interface
// Segregation).
type Marketplace interface {
	Store(ctx context.Context, id string) (Store, error)
	Listings(ctx context.Context, storeID string) ([]Listing, error)
	Listing(ctx context.Context, id string) (Listing, error)
	MedianPrices(ctx context.Context) (map[string]int, error)
	ProtectedMarks(ctx context.Context) ([]Mark, error)
}

// StoreAudit scores how ready a store is to sell and says what to fix.
type StoreAudit struct {
	StoreID       string   `json:"storeId"`
	StoreName     string   `json:"storeName"`
	CustomerEmail string   `json:"customerEmail"`
	CustomerName  string   `json:"customerName"`
	DaysOpen      int      `json:"daysOpen"`
	LiveListings  int      `json:"liveListings"`
	Views30d      int      `json:"views30d"`
	UnitsSold     int      `json:"unitsSold"`
	Score         int      `json:"launchScore"` // 0 to 100
	Tips          []string `json:"tips"`
	NotNeeded     []string `json:"notNeeded"`
}

// ReviewStore checks a store against what the stores that sell have in
// common. Exported for tests and reuse.
func ReviewStore(s Store, listings []Listing, medians map[string]int, now time.Time) StoreAudit {
	a := StoreAudit{
		StoreID: s.ID, StoreName: s.Name, CustomerEmail: s.OwnerEmail, CustomerName: s.OwnerName,
		DaysOpen: int(now.Sub(s.OpenedAt).Hours() / 24), Views30d: s.Views30d, Score: 100,
		Tips: []string{}, NotNeeded: []string{},
	}
	var untagged, pricey []string
	for _, l := range listings {
		a.UnitsSold += l.UnitsSold
		if l.Status != "LIVE" {
			continue
		}
		a.LiveListings++
		if len(l.Tags) < 3 {
			untagged = append(untagged, l.Title)
		}
		if m := medians[l.Product]; m > 0 && l.PriceCents*2 > m*3 { // over 1.5x the median
			pricey = append(pricey, fmt.Sprintf("%s at %s (similar designs sell for about %s)", l.Title, usd(l.PriceCents), usd(m)))
		}
	}
	tip := func(points int, text string) {
		a.Score -= points
		a.Tips = append(a.Tips, text)
	}
	if a.LiveListings < 3 {
		tip(30, fmt.Sprintf("Add at least three designs so visitors have a choice; the store has %d.", a.LiveListings))
	}
	if !s.HasBanner {
		tip(15, "Add a banner image; without one the store page looks unfinished.")
	}
	if len(strings.TrimSpace(s.Description)) < 40 {
		tip(15, "Write a store description of a sentence or two: who you are and what you make.")
	}
	if len(untagged) > 0 {
		tip(min(15, 5*len(untagged)), "Give each design three or more search tags: "+strings.Join(untagged, ", ")+".")
	}
	if len(pricey) > 0 {
		tip(min(20, 10*len(pricey)), "Consider lower prices for "+strings.Join(pricey, "; ")+".")
	}
	if s.Views30d == 0 {
		tip(10, "Nobody has visited in 30 days. Share the store link wherever your audience already is.")
	}
	a.Score = max(a.Score, 0)
	if a.UnitsSold > 0 || len(a.Tips) == 0 { // selling already, or nothing to suggest
		a.NotNeeded = append(a.NotNeeded, "queue_email")
	}
	return a
}

// AuditStore reviews a store that has not made its first sale.
func AuditStore(m Marketplace) Tool {
	return Func(domain.ToolSpec{
		Name: "audit_store",
		Description: "Audit a marketplace store that has not sold yet: designs, tags, prices, banner, description and traffic. " +
			"Returns a launch score, concrete tips and the seller's contact details.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"storeId": {"type": "string", "pattern": "^S-[0-9]{1,8}$"}},
		  "required": ["storeId"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		StoreID string `json:"storeId"`
	}) (any, error) {
		store, err := m.Store(ctx, in.StoreID)
		if err != nil {
			return nil, err
		}
		listings, err := m.Listings(ctx, in.StoreID)
		if err != nil {
			return nil, err
		}
		medians, err := m.MedianPrices(ctx)
		if err != nil {
			return nil, err
		}
		return ReviewStore(store, listings, medians, time.Now()), nil
	})
}

// riskyClaims imply a license the seller would have to prove.
var riskyClaims = []string{"official", "licensed", "replica", "bootleg", "knockoff"}

// Screening is the verdict on a new listing.
type Screening struct {
	ListingID string   `json:"listingId"`
	StoreID   string   `json:"storeId"`
	Title     string   `json:"title"`
	Verdict   string   `json:"verdict"` // CLEAR or NEEDS_REVIEW
	Reasons   []string `json:"reasons"`
	NotNeeded []string `json:"notNeeded"`
}

// CheckListing checks a listing's words against the protected marks
// register. Matching the image itself is a job for a vision model and is
// not part of this rule set. Exported for tests and reuse.
func CheckListing(l Listing, marks []Mark) Screening {
	text := strings.ToLower(l.Title + " \n " + l.Description + " \n " + strings.Join(l.Tags, " \n "))
	s := Screening{ListingID: l.ID, StoreID: l.StoreID, Title: l.Title, Reasons: []string{}}
	for _, m := range marks {
		if containsTerm(text, m.Term) {
			s.Reasons = append(s.Reasons, fmt.Sprintf("mentions %q, a mark owned by %s", m.Term, m.Owner))
		}
	}
	for _, w := range riskyClaims {
		if containsTerm(text, w) {
			s.Reasons = append(s.Reasons, fmt.Sprintf("says %q, which needs proof of a license", w))
		}
	}
	if len(s.Reasons) == 0 {
		s.Verdict, s.NotNeeded = "CLEAR", []string{"notify_team"}
	} else {
		s.Verdict, s.NotNeeded = "NEEDS_REVIEW", []string{"publish_listing"}
	}
	return s
}

func containsTerm(text, term string) bool {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(strings.ToLower(term)) + `\b`).MatchString(text)
}

const listingIDSchema = `{"type": "string", "pattern": "^L-[0-9]{1,8}$"}`

// ScreenListing screens a design submitted to the marketplace.
func ScreenListing(m Marketplace) Tool {
	return Func(domain.ToolSpec{
		Name: "screen_listing",
		Description: "Screen a design submitted to the marketplace for protected brands and characters or claims of a license. " +
			"Verdict CLEAR means it can be published; NEEDS_REVIEW means a person must decide.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"listingId": ` + listingIDSchema + `},
		  "required": ["listingId"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		ListingID string `json:"listingId"`
	}) (any, error) {
		return screen(ctx, m, in.ListingID)
	})
}

func screen(ctx context.Context, m Marketplace, id string) (Screening, error) {
	listing, err := m.Listing(ctx, id)
	if err != nil {
		return Screening{}, err
	}
	marks, err := m.ProtectedMarks(ctx)
	if err != nil {
		return Screening{}, err
	}
	return CheckListing(listing, marks), nil
}

// PublishListing puts a screened design on sale. It screens again itself,
// so no instruction or model mistake can publish a flagged design: the
// guardrail lives in the tool, not in the prompt.
func PublishListing(c Commerce, m Marketplace) Tool {
	return Func(domain.ToolSpec{
		Name:        "publish_listing",
		Description: "Publish a marketplace listing that screened CLEAR. Listings that need review are refused.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"listingId": ` + listingIDSchema + `},
		  "required": ["listingId"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		ListingID string `json:"listingId"`
	}) (any, error) {
		s, err := screen(ctx, m, in.ListingID)
		if err != nil {
			return nil, err
		}
		if s.Verdict != "CLEAR" {
			return nil, fmt.Errorf("listing %s needs review (%s); a person must publish it", s.ListingID, strings.Join(s.Reasons, "; "))
		}
		id, err := c.Record(ctx, "LISTING_PUBLISH", idempotencyKey(ctx, "publish"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"publishId": id, "listingId": s.ListingID, "listingStatus": "LIVE"}, nil
	})
}

func usd(cents int) string { return fmt.Sprintf("$%.2f", float64(cents)/100) }

// ---- demo tables

func (c *PGCommerce) Store(ctx context.Context, id string) (Store, error) {
	var s Store
	err := c.pool.QueryRow(ctx, `
		SELECT id, name, owner_email, owner_name, description, has_banner, views_30d, opened_at
		FROM marketplace_stores WHERE id = $1`, id).
		Scan(&s.ID, &s.Name, &s.OwnerEmail, &s.OwnerName, &s.Description, &s.HasBanner, &s.Views30d, &s.OpenedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Store{}, fmt.Errorf("store %s: %w", id, ErrNotFound)
	}
	return s, err
}

const listingColumns = `id, store_id, title, description, tags, product, price_cents, status, units_sold`

func scanListing(row pgx.CollectableRow) (Listing, error) {
	var l Listing
	err := row.Scan(&l.ID, &l.StoreID, &l.Title, &l.Description, &l.Tags, &l.Product, &l.PriceCents, &l.Status, &l.UnitsSold)
	return l, err
}

func (c *PGCommerce) Listings(ctx context.Context, storeID string) ([]Listing, error) {
	rows, err := c.pool.Query(ctx, `SELECT `+listingColumns+` FROM marketplace_listings WHERE store_id = $1 ORDER BY id`, storeID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanListing)
}

func (c *PGCommerce) Listing(ctx context.Context, id string) (Listing, error) {
	rows, err := c.pool.Query(ctx, `SELECT `+listingColumns+` FROM marketplace_listings WHERE id = $1`, id)
	if err != nil {
		return Listing{}, err
	}
	l, err := pgx.CollectExactlyOneRow(rows, scanListing)
	if errors.Is(err, pgx.ErrNoRows) {
		return Listing{}, fmt.Errorf("listing %s: %w", id, ErrNotFound)
	}
	return l, err
}

func (c *PGCommerce) MedianPrices(ctx context.Context) (map[string]int, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT product, percentile_disc(0.5) WITHIN GROUP (ORDER BY price_cents)
		FROM marketplace_listings WHERE status = 'LIVE' GROUP BY product`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var product string
		var cents int
		if err := rows.Scan(&product, &cents); err != nil {
			return nil, err
		}
		out[product] = cents
	}
	return out, rows.Err()
}

func (c *PGCommerce) ProtectedMarks(ctx context.Context) ([]Mark, error) {
	rows, err := c.pool.Query(ctx, `SELECT term, owner FROM protected_marks ORDER BY term`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Mark])
}
