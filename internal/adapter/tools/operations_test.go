package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/tools"
	"github.com/davidmm07/pressroom/internal/port"
)

func TestAssessClaim(t *testing.T) {
	order := tools.Order{ID: "ORD-1050", CustomerName: "Tom Becker", Quantity: 500, TotalCents: 14510} // 29.02 cents a unit
	clear := func(defect string) tools.Damage { return tools.Damage{Defect: defect, Confidence: 0.93} }
	tests := []struct {
		name      string
		damage    tools.Damage
		in        tools.ClaimInput
		remedy    string
		units     int
		cents     int
		notNeeded string
	}{
		{"misprint is reprinted and reported", clear("MISPRINT"), tools.ClaimInput{ClaimedUnits: 40}, "REPRINT", 40, 0, "issue_refund"},
		{"transit damage skips the team", clear("CRACKED"), tools.ClaimInput{ClaimedUnits: 2}, "REPRINT", 2, 0, "issue_refund,notify_team"},
		{"refund rounds up in the customer's favour", clear("MISCUT"), tools.ClaimInput{ClaimedUnits: 3, PreferredRemedy: "REFUND"}, "REFUND", 0, 88, "order_reprint"},
		{"claims are capped at the order quantity", clear("BENT"), tools.ClaimInput{ClaimedUnits: 9000}, "REPRINT", 500, 0, "issue_refund,notify_team"},
		{"no visible damage goes to a person", clear("NONE_VISIBLE"), tools.ClaimInput{ClaimedUnits: 5}, "HUMAN_REVIEW", 0, 0, "order_reprint,issue_refund"},
		{"a blurry photo goes to a person", tools.Damage{Defect: "MISPRINT", Confidence: 0.4}, tools.ClaimInput{ClaimedUnits: 5}, "HUMAN_REVIEW", 0, 0, "order_reprint,issue_refund"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tools.AssessClaim(order, tt.damage, tt.in)
			if got.Remedy != tt.remedy || got.ReprintUnits != tt.units || got.AmountCents != tt.cents || strings.Join(got.NotNeeded, ",") != tt.notNeeded {
				t.Fatalf("got %+v", got)
			}
		})
	}

	big := tools.Order{ID: "ORD-9", Quantity: 10, TotalCents: 900_000}
	if got := tools.AssessClaim(big, clear("MISPRINT"), tools.ClaimInput{ClaimedUnits: 10, PreferredRemedy: "REFUND"}); got.Remedy != "HUMAN_REVIEW" {
		t.Fatalf("a refund above the agent limit must go to a person, got %+v", got)
	}
}

func TestSandboxVisionReadsTheFileName(t *testing.T) {
	v := tools.SandboxVision{}
	for url, want := range map[string]string{
		"https://cdn.example.com/claims/misprint-colors.jpg": "MISPRINT",
		"https://cdn.example.com/claims/cracked-magnets.jpg": "CRACKED",
		"https://cdn.example.com/claims/IMG_2041.jpg":        "NONE_VISIBLE",
	} {
		if d, _ := v.InspectDamage(context.Background(), url, "STICKERS"); d.Defect != want {
			t.Errorf("%s: got %s, want %s", url, d.Defect, want)
		}
	}
	if d, _ := v.InspectDamage(context.Background(), "https://cdn.example.com/blurry-crack.jpg", "MAGNETS"); d.Confidence >= 0.7 {
		t.Fatalf("a blurry photo must be low confidence, got %+v", d)
	}
}

func TestCheckStyle(t *testing.T) {
	ok := tools.CheckStyle(tools.StyleInput{Text: "Hi Tom, we are reprinting 40 stickers today and they ship free.", CustomerName: "Tom Becker"})
	if !ok.OK || len(ok.Issues) != 0 {
		t.Fatalf("clean reply flagged: %+v", ok)
	}
	bad := tools.CheckStyle(tools.StyleInput{Text: "Sorry, unfortunately we can’t do that.", CustomerName: "Tom Becker"})
	want := []string{`"sorry"`, `"unfortunately"`, `"can't"`, "(Tom)"}
	for _, w := range want {
		if !strings.Contains(strings.Join(bad.Issues, " | "), w) {
			t.Errorf("issues %v do not mention %s", bad.Issues, w)
		}
	}
	long := tools.CheckStyle(tools.StyleInput{Text: strings.Repeat("word ", 130)})
	if long.OK || long.WordCount != 130 {
		t.Fatalf("a 130-word reply must be flagged: %+v", long)
	}
}

func TestCheckEdits(t *testing.T) {
	if err := tools.CheckEdits("STICKERS", []tools.Edit{{Type: "CHANGE_SHAPE", Shape: "CIRCLE"}, {Type: "SET_BORDER", Border: "NONE"}, {Type: "RESIZE", WidthIn: 3, HeightIn: 3}}); err != nil {
		t.Fatalf("valid sticker edits refused: %v", err)
	}
	err := tools.CheckEdits("BUTTONS", []tools.Edit{{Type: "CHANGE_SHAPE", Shape: "SQUARE"}, {Type: "SET_BORDER", Border: "NONE"}, {Type: "RESIZE", WidthIn: 4, HeightIn: 4}})
	for _, want := range []string{"edits[0]", "edits[1]", "edits[2]"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want every bad edit reported, missing %s in %v", want, err)
		}
	}
}

func TestApplyProofRevision(t *testing.T) {
	r, c := registry(t)
	ctx := context.Background()

	_, err := r.Invoke(ctx, "apply_proof_revision", json.RawMessage(`{"artworkUrl":"https://cdn.example.com/a.png","product":"STICKERS","edits":[{"type":"RESIZE","widthIn":3}]}`))
	var inputErr *port.ToolInputError
	if !errors.As(err, &inputErr) || !strings.Contains(err.Error(), "heightIn") {
		t.Fatalf("a resize without a height must fail the schema, got %v", err)
	}

	out, err := r.Invoke(ctx, "apply_proof_revision", json.RawMessage(`{"artworkUrl":"https://cdn.example.com/a.png","product":"STICKERS",
		"edits":[{"type":"CHANGE_SHAPE","shape":"CIRCLE"},{"type":"SET_BORDER","border":"NONE"}],"designerRequest":"Make the text bolder"}`))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ArtworkURL    string   `json:"artworkUrl"`
		Applied       []string `json:"applied"`
		NeedsDesigner bool     `json:"needsDesigner"`
	}
	_ = json.Unmarshal(out, &got)
	if !strings.Contains(got.ArtworkURL, "shape=CIRCLE") || !strings.Contains(got.ArtworkURL, "border=NONE") || len(got.Applied) != 2 || !got.NeedsDesigner || len(c.records) != 1 {
		t.Fatalf("got %s", out)
	}

	if _, err := r.Invoke(ctx, "apply_proof_revision", json.RawMessage(`{"artworkUrl":"https://cdn.example.com/a.png","product":"STICKERS","edits":[]}`)); err == nil {
		t.Fatal("a revision with nothing to do must fail")
	}
}

type fakeMarketplace struct {
	store    tools.Store
	listings []tools.Listing
}

func (f fakeMarketplace) Store(context.Context, string) (tools.Store, error) { return f.store, nil }
func (f fakeMarketplace) Listings(context.Context, string) ([]tools.Listing, error) {
	return f.listings, nil
}
func (f fakeMarketplace) Listing(_ context.Context, id string) (tools.Listing, error) {
	for _, l := range f.listings {
		if l.ID == id {
			return l, nil
		}
	}
	return tools.Listing{}, tools.ErrNotFound
}
func (f fakeMarketplace) MedianPrices(context.Context) (map[string]int, error) {
	return map[string]int{"STICKERS": 700}, nil
}
func (f fakeMarketplace) ProtectedMarks(context.Context) ([]tools.Mark, error) {
	return []tools.Mark{{Term: "Galaxy Rangers", Owner: "Starfield Studios"}}, nil
}

func TestReviewStore(t *testing.T) {
	now := time.Now().UTC()
	store := tools.Store{ID: "S-201", OwnerName: "Kai Morgan", Description: "Stickers.", OpenedAt: now.AddDate(0, 0, -24)}
	listings := []tools.Listing{
		{Title: "Sleepy owl", Tags: []string{"owl"}, Product: "STICKERS", PriceCents: 900, Status: "LIVE"},
		{Title: "Moon phases", Tags: []string{"moon", "space", "night"}, Product: "STICKERS", PriceCents: 2500, Status: "LIVE"},
	}
	a := tools.ReviewStore(store, listings, map[string]int{"STICKERS": 700}, now)
	// 100 - 30 (two designs) - 15 (banner) - 15 (description) - 5 (tags) - 10 (price) - 10 (no visits)
	if a.Score != 15 || len(a.Tips) != 6 || a.DaysOpen != 24 || len(a.NotNeeded) != 0 {
		t.Fatalf("got %+v", a)
	}
	if !strings.Contains(strings.Join(a.Tips, " "), "Moon phases at $25.00") {
		t.Fatalf("the overpriced design is not named: %v", a.Tips)
	}

	listings[0].UnitsSold = 1
	if a := tools.ReviewStore(store, listings, nil, now); strings.Join(a.NotNeeded, ",") != "queue_email" {
		t.Fatalf("a store that sells needs no coaching email: %+v", a)
	}
}

func TestListingsAreScreenedBeforeTheyArePublished(t *testing.T) {
	flagged := tools.Listing{ID: "L-1", Title: "GALAXY RANGERS fan art", Tags: []string{"space"}, Status: "IN_REVIEW"}
	clean := tools.Listing{ID: "L-2", Title: "Muddy boots", Tags: []string{"hiking"}, Status: "IN_REVIEW"}
	official := tools.Listing{ID: "L-3", Title: "Official team logo", Status: "IN_REVIEW"}
	marks := []tools.Mark{{Term: "Galaxy Rangers", Owner: "Starfield Studios"}}

	if s := tools.CheckListing(flagged, marks); s.Verdict != "NEEDS_REVIEW" || !strings.Contains(s.Reasons[0], "Starfield Studios") {
		t.Fatalf("protected mark missed: %+v", s)
	}
	if s := tools.CheckListing(official, marks); s.Verdict != "NEEDS_REVIEW" {
		t.Fatalf("claim of a license missed: %+v", s)
	}
	if s := tools.CheckListing(clean, marks); s.Verdict != "CLEAR" || strings.Join(s.NotNeeded, ",") != "notify_team" {
		t.Fatalf("clean listing flagged: %+v", s)
	}

	c := &fakeCommerce{}
	r, err := tools.Standard(tools.Dependencies{Commerce: c, Marketplace: fakeMarketplace{listings: []tools.Listing{flagged, clean}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Invoke(context.Background(), "publish_listing", json.RawMessage(`{"listingId":"L-1"}`)); err == nil || !strings.Contains(err.Error(), "needs review") {
		t.Fatalf("a flagged listing must never be published, got %v", err)
	}
	if _, err := r.Invoke(context.Background(), "publish_listing", json.RawMessage(`{"listingId":"L-2"}`)); err != nil || len(c.records) != 1 {
		t.Fatalf("clean listing not published: %v", err)
	}
}

var (
	today    = time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	stations = []tools.Station{
		{ID: "PRESS-1", Products: []string{"STICKERS", "LABELS"}, UnitsPerDay: 4000},
		{ID: "PRESS-2", Products: []string{"STICKERS", "LABELS", "MAGNETS"}, UnitsPerDay: 3000},
		{ID: "PRESS-3", Products: []string{"MAGNETS", "BUTTONS"}, UnitsPerDay: 1500},
	}
)

func TestPlanJob(t *testing.T) {
	hour := func(h int) time.Time { return today.Add(time.Duration(h) * time.Hour) }
	late := tools.Job{ID: "J-4", Product: "LABELS", Quantity: 1000, Station: "PRESS-1", Status: "QUEUED", QueuedAt: hour(-20), ShipBy: today.AddDate(0, 0, 2)}
	open := []tools.Job{
		{ID: "J-1", Product: "STICKERS", Quantity: 6000, Station: "PRESS-1", QueuedAt: hour(-50)},
		{ID: "J-2", Product: "LABELS", Quantity: 5000, Station: "PRESS-1", QueuedAt: hour(-40)},
		{ID: "J-3", Product: "STICKERS", Quantity: 4000, Station: "PRESS-1", QueuedAt: hour(-30)},
		late,
		{ID: "J-5", Product: "MAGNETS", Quantity: 1500, Station: "PRESS-2", QueuedAt: hour(-26)},
		{ID: "J-9", Product: "STICKERS", Quantity: 9000, Station: "PRESS-1", QueuedAt: hour(-1)}, // behind it
	}

	p := tools.PlanJob(late, stations, open, today.Add(9*time.Hour))
	if !p.Late || p.ProjectedDoneOn != "2026-10-03" || p.TargetStation != "PRESS-2" || p.TargetDoneOn != "2026-09-30" || len(p.NotNeeded) != 0 {
		t.Fatalf("late job: %+v", p)
	}

	onTime := late
	onTime.ShipBy = today.AddDate(0, 0, 5)
	if p := tools.PlanJob(onTime, stations, open, today); p.Late || strings.Join(p.NotNeeded, ",") != "reroute_job,notify_team" {
		t.Fatalf("on-time job: %+v", p)
	}

	nowhere := late
	nowhere.ShipBy = today // nothing finishes today
	if p := tools.PlanJob(nowhere, stations, open, today); !p.Late || p.TargetStation != "" || strings.Join(p.NotNeeded, ",") != "reroute_job" {
		t.Fatalf("hopeless job: %+v", p)
	}
}

type fakeFactory struct{ job tools.Job }

func (f fakeFactory) Job(context.Context, string) (tools.Job, error)    { return f.job, nil }
func (f fakeFactory) Stations(context.Context) ([]tools.Station, error) { return stations, nil }
func (f fakeFactory) OpenJobs(context.Context) ([]tools.Job, error)     { return []tools.Job{f.job}, nil }

func TestRerouteJobOnlyToAStationThatPrintsTheProduct(t *testing.T) {
	c := &fakeCommerce{}
	f := fakeFactory{job: tools.Job{ID: "J-4", Product: "LABELS", Station: "PRESS-1"}}
	r, err := tools.Standard(tools.Dependencies{Commerce: c, Factory: f})
	if err != nil {
		t.Fatal(err)
	}
	for station, want := range map[string]string{"PRESS-3": "does not print LABELS", "PRESS-1": "already on", "PRESS-9": "not found"} {
		args := json.RawMessage(`{"jobId":"J-4","targetStation":"` + station + `","reason":"it would ship late"}`)
		if _, err := r.Invoke(context.Background(), "reroute_job", args); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want %q, got %v", station, want, err)
		}
	}
	if _, err := r.Invoke(context.Background(), "reroute_job", json.RawMessage(`{"jobId":"J-4","targetStation":"PRESS-2","reason":"it would ship late"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestForecastSuggestsADealForTheIdlestLine(t *testing.T) {
	open := []tools.Job{{Product: "STICKERS", Quantity: 10_000}, {Product: "LABELS", Quantity: 6000}, {Product: "MAGNETS", Quantity: 1500}}
	f := tools.ForecastCapacity(stations, open, 14)
	d := f.SuggestedDeal
	if d == nil || d.Product != "LABELS" || d.FloorCents != 1125 || d.PriceCents != 1500 || d.PriceCents < tools.DealFloorCents("LABELS", d.PackSize) {
		t.Fatalf("got %+v", f)
	}

	busy := []tools.Job{{Product: "STICKERS", Quantity: 40_000}, {Product: "LABELS", Quantity: 40_000}, {Product: "MAGNETS", Quantity: 24_000}, {Product: "BUTTONS", Quantity: 10_000}}
	if f := tools.ForecastCapacity(stations, busy, 14); f.SuggestedDeal != nil || strings.Join(f.NotNeeded, ",") != "propose_deal" {
		t.Fatalf("no deal when the presses are busy: %+v", f)
	}
}

func TestProposeDealKeepsTheMargin(t *testing.T) {
	r, _ := registry(t)
	below := json.RawMessage(`{"product":"LABELS","packSize":50,"priceCents":1000,"days":5,"headline":"50 custom labels for $10"}`)
	if _, err := r.Invoke(context.Background(), "propose_deal", below); err == nil || !strings.Contains(err.Error(), "below the $11.25 floor") {
		t.Fatalf("got %v", err)
	}
	out, err := r.Invoke(context.Background(), "propose_deal", json.RawMessage(`{"product":"LABELS","packSize":50,"priceCents":1500,"days":5,"headline":"50 custom labels for $15"}`))
	if err != nil || !strings.Contains(string(out), `"marginPercent":40`) {
		t.Fatalf("got %s, %v", out, err)
	}
}

func TestDigestReviews(t *testing.T) {
	topics := map[string]string{
		"Corners started peeling after a week": "ADHESION",
		"Every one of them fell off my laptop": "ADHESION",
		"The red came out faded":               "COLOR",
		"Super cute, love them":                "OTHER", // not CUTTING
		"The cut is crooked on half of them":   "CUTTING",
		"One magnet arrived bent":              "DAMAGE",
		"Took two weeks, shipping was so slow": "SHIPPING",
		"They won’t stick to anything cold":    "ADHESION",
	}
	for body, want := range topics {
		if got := tools.ReviewTopic(body); got != want {
			t.Errorf("%q: got %s, want %s", body, got, want)
		}
	}

	var reviews []tools.Review
	for i, body := range []string{"peeling", "peeling edges", "fell off", "adhesive is weak", "faded", "bent"} {
		product := "STICKERS"
		if i >= 4 {
			product = "LABELS"
		}
		reviews = append(reviews, tools.Review{OrderID: "ORD-10" + string(rune('0'+i)) + "0", Product: product, Body: body})
	}
	d := tools.DigestReviews(reviews)
	if d.TopIssue == nil || d.TopIssue.Product != "STICKERS" || d.TopIssue.Category != "ADHESION" || d.TopIssue.ReviewCount != 4 || len(d.NotNeeded) != 0 {
		t.Fatalf("got %+v", d)
	}
	if d := tools.DigestReviews(reviews[4:]); d.TopIssue != nil || len(d.NotNeeded) != 2 {
		t.Fatalf("one-off complaints are not a pattern: %+v", d)
	}
}

func TestScreenEntries(t *testing.T) {
	if got := tools.NormalizeEmail("A.Na.Lopez+Giveaway@GoogleMail.com"); got != "analopez@gmail.com" {
		t.Errorf("NormalizeEmail = %s", got)
	}
	if got := tools.NormalizeAddress("12 Oak Street, Apt. 4, Boston MA"); got != "12 oak st apt 4 boston ma" {
		t.Errorf("NormalizeAddress = %s", got)
	}
	entries := []tools.Entry{
		{Email: "ana.lopez@gmail.com", Address: "221 Pine Road"},
		{Email: "a.na.lopez+win@gmail.com", Address: "221 Pine Rd"},
		{Email: "win@mailinator.com", Address: "400 Elm Street"},
		{Email: "sara@example.com", Address: "12 Oak Street, Apt. 4"},
		{Email: "s.alt@example.com", Address: "12 oak st apt 4"},
		{Email: "joe@example.com", Address: "77 Lake Drive"},
	}
	s := tools.ScreenEntries("G-12", entries)
	if s.EligibleEntries != 3 || s.DisqualifiedEntries != 3 || s.Reasons["DUPLICATE_EMAIL"] != 1 || s.Reasons["DISPOSABLE_EMAIL"] != 1 || s.Reasons["DUPLICATE_ADDRESS"] != 1 {
		t.Fatalf("got %+v", s)
	}
	for _, ex := range s.Examples {
		if strings.Contains(ex, "lopez") || strings.Contains(ex, "Elm") {
			t.Fatalf("examples must be masked: %v", s.Examples)
		}
	}
}
