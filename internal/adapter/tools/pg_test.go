package tools_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/postgres"
	"github.com/davidmm07/pressroom/internal/adapter/tools"
	"github.com/davidmm07/pressroom/internal/domain"
)

// TestPGCommerceReadsTheDemoTables runs the demo adapter's queries against
// a real database when PRESSROOM_TEST_DATABASE_URL is set. Rows get a
// per-run suffix, so the test never trips over earlier runs.
func TestPGCommerceReadsTheDemoTables(t *testing.T) {
	url := os.Getenv("PRESSROOM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PRESSROOM_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := postgres.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}

	id := string(domain.NewID())[26:] // unique per run
	product := "TEST_" + strings.ToUpper(id)
	now := time.Now().UTC()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO marketplace_stores (id, name, owner_email, owner_name, opened_at) VALUES ($1, 'Test store', 'seller@example.com', 'Sam Seller', $2)`, "S-"+id, now)
	for i, price := range []int{500, 900, 700} {
		exec(`INSERT INTO marketplace_listings (id, store_id, title, tags, product, price_cents, status) VALUES ($1, $2, 'Design', '{a,b}', $3, $4, 'LIVE')`,
			"L-"+id+string(rune('a'+i)), "S-"+id, product, price)
	}
	exec(`INSERT INTO protected_marks (term, owner) VALUES ($1, 'Test Owner')`, "Mark "+id)
	exec(`INSERT INTO production_stations (id, products, units_per_day) VALUES ($1, $2, 100)`, "T-"+id, []string{product})
	exec(`INSERT INTO production_jobs (id, order_id, product, quantity, station_id, status, queued_at, ship_by) VALUES
		($1, 'ORD-1', $2, 50, $3, 'QUEUED', $4, $5), ($6, 'ORD-2', $2, 50, $3, 'DONE', $4, $5)`,
		"J-"+id, product, "T-"+id, now, now.AddDate(0, 0, 3), "J-"+id+"x")
	exec(`INSERT INTO storefront_reviews (id, order_id, product, rating, body, created_at) VALUES
		($1, 'ORD-1', $2, 2, 'peeling', $3), ($4, 'ORD-2', $2, 5, 'great', $3), ($5, 'ORD-3', $2, 1, 'old', $6)`,
		"R-"+id, product, now, "R-"+id+"x", "R-"+id+"y", now.AddDate(0, 0, -30))
	exec(`INSERT INTO giveaway_entries (giveaway_id, email, name, address, entered_at) VALUES ($1, 'b@example.com', 'B', '1 Road', $2), ($1, 'a@example.com', 'A', '2 Road', $3)`,
		"G-"+id, now, now.Add(-time.Hour))

	c := tools.NewPGCommerce(pool)
	if s, err := c.Store(ctx, "S-"+id); err != nil || s.OwnerName != "Sam Seller" {
		t.Fatalf("Store = %+v, %v", s, err)
	}
	if ls, err := c.Listings(ctx, "S-"+id); err != nil || len(ls) != 3 || strings.Join(ls[0].Tags, ",") != "a,b" {
		t.Fatalf("Listings = %+v, %v", ls, err)
	}
	if _, err := c.Listing(ctx, "L-missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Listing on a missing id: %v", err)
	}
	if m, err := c.MedianPrices(ctx); err != nil || m[product] != 700 {
		t.Fatalf("MedianPrices = %v, %v", m[product], err)
	}
	if marks, err := c.ProtectedMarks(ctx); err != nil || !slices.Contains(marks, tools.Mark{Term: "Mark " + id, Owner: "Test Owner"}) {
		t.Fatalf("ProtectedMarks = %v, %v", marks, err)
	}
	if j, err := c.Job(ctx, "J-"+id); err != nil || j.Station != "T-"+id || j.ShipBy.Format(time.DateOnly) != now.AddDate(0, 0, 3).Format(time.DateOnly) {
		t.Fatalf("Job = %+v, %v", j, err)
	}
	open, err := c.OpenJobs(ctx)
	if err != nil || !slices.ContainsFunc(open, func(j tools.Job) bool { return j.ID == "J-"+id }) ||
		slices.ContainsFunc(open, func(j tools.Job) bool { return j.ID == "J-"+id+"x" }) {
		t.Fatalf("OpenJobs must list the queued job and skip the finished one: %v", err)
	}
	if st, err := c.Stations(ctx); err != nil || !slices.ContainsFunc(st, func(s tools.Station) bool { return s.ID == "T-"+id && s.UnitsPerDay == 100 }) {
		t.Fatalf("Stations = %v, %v", st, err)
	}
	reviews, err := c.Recent(ctx, now.AddDate(0, 0, -7), 3)
	mine := slices.DeleteFunc(reviews, func(r tools.Review) bool { return r.Product != product })
	if err != nil || len(mine) != 1 || mine[0].ID != "R-"+id {
		t.Fatalf("Recent = %+v, %v", mine, err)
	}
	if e, err := c.Entries(ctx, "G-"+id); err != nil || len(e) != 2 || e[0].Email != "a@example.com" {
		t.Fatalf("Entries must be oldest first: %+v, %v", e, err)
	}
}
