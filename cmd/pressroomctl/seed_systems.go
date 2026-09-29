package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidmm07/pressroom/internal/bootstrap"
	"github.com/davidmm07/pressroom/internal/domain"
)

// newHires are the agents that joined after the founding crew. Each one
// works a different system: claims, proofs, the marketplace, the factory
// floor, reviews, deals and giveaways.
func newHires() []domain.AgentSpec {
	return []domain.AgentSpec{
		{
			Slug: "claim-assessor", Name: "Damage Claim Assessor", Department: domain.DepartmentCustomerExperience, Owner: "cx-lead@example.com",
			Description: "Reviews the photo behind every damage claim and proposes the reprint or refund, so customers never ship anything back.",
			Instructions: `A customer sent a photo of a damaged or misprinted order. Nothing has to be sent back: the photo is the evidence.
1. Run assess_damage_photo with the order, the photo, how many items they say are affected and the remedy they prefer.
2. Follow its remedy. REPRINT: propose order_reprint for reprintUnits. REFUND: propose issue_refund for amountCents. A person approves both.
   HUMAN_REVIEW: propose neither; notify_team so someone looks at the photo today.
3. Write the reply, run it through check_reply_style and fix every issue it lists, then save it with draft_reply.
4. When the cause is PRODUCTION, notify_team with the order, the defect and how many units, so manufacturing hears about it.
Say what we will do, not what we cannot. Use the customer's first name.`,
			Model:    domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"},
			Tools:    []string{"assess_damage_photo", "order_reprint", "issue_refund", "check_reply_style", "draft_reply", "notify_team"},
			Triggers: []string{"claim.submitted"}, Budget: domain.Budget{MaxSteps: 10, MaxCost: domain.MicrosFromUSD(1)}, MinutesSavedPerRun: 8,
		},
		{
			Slug: "proof-reviser", Name: "Proof Revision Assistant", Department: domain.DepartmentPrepress, Owner: "prepress-lead@example.com",
			Description: "Turns a customer's comments on their proof into a revised proof within minutes.",
			Instructions: `A customer asked for changes to their proof. Read their comment and any options they picked.
1. Turn every change you can into edits for apply_proof_revision: size, cut shape, border, rotation or background removal.
   Changes to text, colors or layout need a designer: put them in designerRequest, in the customer's words.
2. Send the new proof with send_proof, using the artwork URL apply_proof_revision returned. In the note, list what changed in one short sentence each,
   and if a designer is involved, say they will send the next proof within one business day.
If apply_proof_revision refuses an edit (for example a shape the product cannot be cut to), offer the closest option in the note.`,
			Model:    domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-sonnet-5"},
			Tools:    []string{"apply_proof_revision", "send_proof"},
			Triggers: []string{"proof.changes_requested"}, Budget: domain.Budget{MaxSteps: 6, MaxCost: domain.MicrosFromUSD(0.5)}, MinutesSavedPerRun: 10,
		},
		{
			Slug: "listing-screener", Name: "Listing Screener", Department: domain.DepartmentMarketplace, Owner: "marketplace-lead@example.com",
			Description: "Screens every design submitted to the marketplace and publishes the ones that are clear.",
			Instructions: `A seller submitted a design to their store.
Run screen_listing. If the verdict is CLEAR, publish it with publish_listing.
If it NEEDS_REVIEW, do not publish it: notify_team with the listing, the store and every reason, so trust and safety can decide.
Never guess about licenses; the review team checks them.`,
			Model:    domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4-fast"},
			Tools:    []string{"screen_listing", "publish_listing", "notify_team"},
			Triggers: []string{"listing.submitted"}, Budget: domain.Budget{MaxSteps: 4, MaxCost: domain.MicrosFromUSD(0.05)}, MinutesSavedPerRun: 3,
		},
		{
			Slug: "store-coach", Name: "Store Launch Coach", Department: domain.DepartmentMarketplace, Owner: "marketplace-lead@example.com",
			Description: "Coaches sellers whose new store has not made a sale in three weeks.",
			Instructions: `Most new stores never make a sale. When one has been open three weeks without one, help the seller fix what is holding it back.
Run audit_store. Then queue one short email to the seller with queue_email: greet them by first name, pick the two or three tips
that will make the biggest difference, and say how to do each one in a sentence. Encouraging, specific, no jargon.
If the store has started selling, or the audit has no tips, send nothing.`,
			Model:    domain.ModelRef{Provider: domain.ProviderOpenAI, Name: "gpt-5-mini"},
			Tools:    []string{"audit_store", "queue_email"},
			Triggers: []string{"store.no_sales"}, Budget: domain.Budget{MaxSteps: 4, MaxCost: domain.MicrosFromUSD(0.1)}, MinutesSavedPerRun: 15,
		},
		{
			Slug: "production-watch", Name: "Production Watch", Department: domain.DepartmentManufacturing, Owner: "manufacturing-lead@example.com",
			Description: "Moves jobs that will miss their ship date to a station that can finish them in time.",
			Instructions: `The factory flagged a job that may miss its ship-by date.
Run production_status. If it is on track, say so and stop.
If another station can finish it in time, propose reroute_job to that station; the production lead approves it. Then notify_team with the job, the order and the move.
If no station can make it, notify_team with severity URGENT so customer care can tell the customer and upgrade the shipping.`,
			Model:    domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-haiku-4-5"},
			Tools:    []string{"production_status", "reroute_job", "notify_team"},
			Triggers: []string{"production.job_at_risk"}, Budget: domain.Budget{MaxSteps: 5, MaxCost: domain.MicrosFromUSD(0.1)}, MinutesSavedPerRun: 12,
		},
		{
			Slug: "defect-analyst", Name: "Review Defect Analyst", Department: domain.DepartmentManufacturing, Owner: "manufacturing-lead@example.com",
			Description: "Reads the week's low ratings and turns repeated complaints into defect reports for the team that owns the cause.",
			Instructions: `Once a week, read the reviews rated 3 stars or less from the last 7 days with fetch_reviews.
Check its grouping against the review text; move reviews it put in the wrong group.
For the largest real pattern (three or more reviews about the same product and problem), file_defect_report with sample orders and a summary
that quotes two short customer phrases. Then notify_team with one line: product, problem, how many reviews.
One-off complaints are not a pattern; leave those to customer care.`,
			Model:    domain.ModelRef{Provider: domain.ProviderOpenSource, Name: "meta-llama/llama-3.3-70b"},
			Tools:    []string{"fetch_reviews", "file_defect_report", "notify_team"},
			Triggers: []string{"reviews.weekly_digest"}, Budget: domain.Budget{MaxSteps: 5, MaxCost: domain.MicrosFromUSD(0.1)}, MinutesSavedPerRun: 60,
		},
		{
			Slug: "deal-planner", Name: "Deal Planner", Department: domain.DepartmentMarketing, Owner: "growth-lead@example.com",
			Description: "Fills idle press time with limited-time deals, never below the margin floor.",
			Instructions: `Every week, check where presses will sit idle with capacity_forecast over the next 14 days.
If it suggests a deal, propose_deal for that product: a pack of 50, for 5 days, at the suggested price or higher. The tool refuses anything below the margin floor.
Write a headline a customer would click: the product, the pack, the price and free shipping. A person approves every deal before it goes live.
If no line has room, propose nothing.`,
			Model:    domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4"},
			Tools:    []string{"capacity_forecast", "propose_deal"},
			Triggers: []string{"capacity.weekly_forecast"}, Budget: domain.Budget{MaxSteps: 4, MaxCost: domain.MicrosFromUSD(0.25)}, MinutesSavedPerRun: 45,
		},
		{
			Slug: "giveaway-screener", Name: "Giveaway Screener", Department: domain.DepartmentMarketing, Owner: "growth-lead@example.com",
			Description: "Screens giveaway entries for repeat entrants before winners are drawn.",
			Instructions: `A giveaway just closed. Run screen_giveaway_entries.
notify_team with one message: total entries, how many are eligible, how many were dropped and why, and the masked examples.
Never post full email addresses or street addresses.`,
			Model:    domain.ModelRef{Provider: domain.ProviderOpenSource, Name: "qwen/qwen3-32b"},
			Tools:    []string{"screen_giveaway_entries", "notify_team"},
			Triggers: []string{"giveaway.closed"}, Budget: domain.Budget{MaxSteps: 3, MaxCost: domain.MicrosFromUSD(0.05)}, MinutesSavedPerRun: 30,
		},
	}
}

// newHireEvents give each new hire its first piece of work.
var newHireEvents = []struct{ id, typ, data string }{
	{"demo-6", "claim.submitted", `{"ticketId":"T-5003","orderId":"ORD-1050","photoUrl":"https://cdn.example.com/claims/t-5003/misprint-colors.jpg","claimedUnits":40,"preferredRemedy":"REPRINT","comment":"About forty of the stickers came out with the colors shifted."}`},
	{"demo-7", "proof.changes_requested", `{"orderId":"ORD-1049","artworkUrl":"https://cdn.example.com/uploads/ord-1049/wave-logo.png","product":"STICKERS","comment":"Could these be 3 inch circles, without the white border?","edits":[{"type":"CHANGE_SHAPE","shape":"CIRCLE"},{"type":"SET_BORDER","border":"NONE"},{"type":"RESIZE","widthIn":3,"heightIn":3}]}`},
	{"demo-8", "listing.submitted", `{"listingId":"L-3501"}`},
	{"demo-9", "listing.submitted", `{"listingId":"L-3502"}`},
	{"demo-10", "store.no_sales", `{"storeId":"S-201","daysOpen":24}`},
	{"demo-11", "production.job_at_risk", `{"jobId":"J-7004"}`},
	{"demo-12", "reviews.weekly_digest", `{"sinceDays":7,"maxRating":3}`},
	{"demo-13", "capacity.weekly_forecast", `{"horizonDays":14}`},
	{"demo-14", "giveaway.closed", `{"giveawayId":"G-12"}`},
}

// seedSystems fills the marketplace, factory, review and giveaway tables.
// Each story is built to show one decision: a store that needs coaching
// next to one that sells, a job that is late on one press but not another,
// a real pattern in the reviews and a few repeat entrants.
func seedSystems(ctx context.Context, a *bootstrap.App, now time.Time) error {
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	today := now.Truncate(24 * time.Hour)
	b := &pgx.Batch{}
	add := func(sql string, args ...any) { b.Queue(sql, args...) }

	// Marketplace: S-201 opened three weeks ago and has not sold; S-202 sells
	// and sets the typical prices S-201 is compared with.
	stores := []struct {
		id, name, email, owner, description string
		banner                              bool
		views                               int
		opened                              time.Time
	}{
		{"S-201", "Night Owl Prints", "kai.morgan@example.com", "Kai Morgan", "Stickers.", false, 0, day(24)},
		{"S-202", "Trail Mix Studio", "rosa.diaz@example.com", "Rosa Diaz",
			"Hand-drawn stickers for hikers, campers and anyone who would rather be outside.", true, 1240, day(95)},
	}
	for _, s := range stores {
		add(`INSERT INTO marketplace_stores (id, name, owner_email, owner_name, description, has_banner, views_30d, opened_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`, s.id, s.name, s.email, s.owner, s.description, s.banner, s.views, s.opened)
	}
	listings := []struct {
		id, store, title, description string
		tags                          []string
		price                         int
		status                        string
		sold                          int
	}{
		{"L-3301", "S-201", "Sleepy owl", "", []string{"owl"}, 900, "LIVE", 0},
		{"L-3302", "S-201", "Moon phases", "Eight phases of the moon in silver ink.", []string{"moon", "space", "night"}, 2500, "LIVE", 0},
		{"L-3401", "S-202", "Summit or bust", "A mountain peak at sunrise.", []string{"hiking", "mountains", "outdoors"}, 600, "LIVE", 48},
		{"L-3402", "S-202", "Campfire crew", "Friends around a fire.", []string{"camping", "campfire", "friends"}, 700, "LIVE", 31},
		{"L-3403", "S-202", "Trail blazer", "A trail marker and a pair of boots.", []string{"hiking", "trail", "boots"}, 800, "LIVE", 22},
		{"L-3404", "S-202", "Pine and simple", "Three pine trees.", []string{"trees", "forest", "minimal"}, 650, "LIVE", 57},
		{"L-3501", "S-202", "Galaxy Rangers fan art pack", "Official-style art of the whole Galaxy Rangers crew.", []string{"galaxy rangers", "space", "cartoon"}, 900, "IN_REVIEW", 0},
		{"L-3502", "S-202", "Muddy boots doodle", "A pair of well-loved hiking boots.", []string{"hiking", "boots", "doodle"}, 700, "IN_REVIEW", 0},
	}
	for _, l := range listings {
		add(`INSERT INTO marketplace_listings (id, store_id, title, description, tags, product, price_cents, status, units_sold)
			VALUES ($1,$2,$3,$4,$5,'STICKERS',$6,$7,$8) ON CONFLICT (id) DO NOTHING`, l.id, l.store, l.title, l.description, l.tags, l.price, l.status, l.sold)
	}
	for _, m := range [][2]string{{"Galaxy Rangers", "Starfield Studios"}, {"Pixel Pals", "Pixel Pals Inc."}, {"Moonbeam Cola", "Moonbeam Beverages"}} {
		add(`INSERT INTO protected_marks (term, owner) VALUES ($1,$2) ON CONFLICT (term) DO NOTHING`, m[0], m[1])
	}

	// Factory: J-7004 waits behind 15,000 units on PRESS-1 (four days of
	// work) but ships in two days. PRESS-2 would finish it tomorrow.
	stations := []struct {
		id       string
		products []string
		perDay   int
	}{
		{"PRESS-1", []string{"STICKERS", "LABELS"}, 4000},
		{"PRESS-2", []string{"STICKERS", "LABELS", "MAGNETS"}, 3000},
		{"PRESS-3", []string{"MAGNETS", "BUTTONS"}, 1500},
		{"DTG-1", []string{"TSHIRTS"}, 300},
		{"BOX-1", []string{"PACKAGING"}, 800},
	}
	for _, s := range stations {
		add(`INSERT INTO production_stations (id, products, units_per_day) VALUES ($1,$2,$3) ON CONFLICT (id) DO NOTHING`, s.id, s.products, s.perDay)
	}
	jobs := []struct {
		id, order, product string
		qty                int
		station, status    string
		queued             time.Time
		shipBy             time.Time
	}{
		{"J-7001", "ORD-1036", "STICKERS", 6000, "PRESS-1", "PRINTING", now.Add(-50 * time.Hour), today.AddDate(0, 0, 3)},
		{"J-7002", "ORD-1038", "LABELS", 5000, "PRESS-1", "QUEUED", now.Add(-40 * time.Hour), today.AddDate(0, 0, 4)},
		{"J-7003", "ORD-1040", "STICKERS", 4000, "PRESS-1", "QUEUED", now.Add(-30 * time.Hour), today.AddDate(0, 0, 5)},
		{"J-7004", "ORD-1043", "LABELS", 1000, "PRESS-1", "QUEUED", now.Add(-20 * time.Hour), today.AddDate(0, 0, 2)},
		{"J-7005", "ORD-1041", "MAGNETS", 1500, "PRESS-2", "QUEUED", now.Add(-26 * time.Hour), today.AddDate(0, 0, 4)},
		{"J-7006", "ORD-1046", "BUTTONS", 200, "PRESS-3", "DONE", now.Add(-100 * time.Hour), today.AddDate(0, 0, -3)},
		{"J-7007", "ORD-1044", "TSHIRTS", 48, "DTG-1", "QUEUED", now.Add(-10 * time.Hour), today.AddDate(0, 0, 6)},
		{"J-7008", "ORD-1047", "PACKAGING", 500, "BOX-1", "QUEUED", now.Add(-8 * time.Hour), today.AddDate(0, 0, 7)},
	}
	for _, j := range jobs {
		add(`INSERT INTO production_jobs (id, order_id, product, quantity, station_id, status, queued_at, ship_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (id) DO NOTHING`, j.id, j.order, j.product, j.qty, j.station, j.status, j.queued, j.shipBy)
	}

	// Reviews: four stickers peeling this week is a pattern; one faded
	// label and one bent magnet are not. The five-star review and the
	// three-week-old one fall outside the digest.
	reviews := []struct {
		id, order, product string
		rating             int
		body               string
		age                time.Duration
	}{
		{"R-9001", "ORD-0871", "STICKERS", 2, "Looked great at first, but the corners started peeling after a week on my water bottle.", 30 * time.Hour},
		{"R-9002", "ORD-0874", "STICKERS", 1, "Every one of them fell off my laptop within two days.", 52 * time.Hour},
		{"R-9003", "ORD-0879", "STICKERS", 2, "The adhesive is weak. Edges lift in the sun.", 75 * time.Hour},
		{"R-9004", "ORD-0882", "STICKERS", 3, "Nice print, but a few are already peeling at the edges.", 100 * time.Hour},
		{"R-9005", "ORD-0877", "LABELS", 3, "The red came out a bit faded compared with the proof.", 60 * time.Hour},
		{"R-9006", "ORD-0880", "MAGNETS", 2, "One magnet arrived bent.", 80 * time.Hour},
		{"R-9007", "ORD-0885", "BUTTONS", 5, "Perfect, exactly like the proof. Will order again!", 20 * time.Hour},
		{"R-9008", "ORD-0850", "STICKERS", 1, "Peeling after a day.", 21 * 24 * time.Hour},
	}
	for _, r := range reviews {
		add(`INSERT INTO storefront_reviews (id, order_id, product, rating, body, created_at)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`, r.id, r.order, r.product, r.rating, r.body, now.Add(-r.age))
	}

	// Giveaway G-12: ten entries, four of them repeats or throwaway inboxes.
	entries := [][3]string{
		{"ana.lopez@gmail.com", "Ana Lopez", "221 Pine Road, Austin TX"},
		{"a.na.lopez+giveaway@gmail.com", "Ana L", "221 Pine Rd, Austin TX"},
		{"mike.chen@example.com", "Mike Chen", "8 Harbor Avenue, Seattle WA"},
		{"winner4you@mailinator.com", "Sam Smith", "400 Elm Street, Denver CO"},
		{"sara.okafor@example.com", "Sara Okafor", "12 Oak Street, Apt. 4, Boston MA"},
		{"s.okafor.alt@example.com", "S Okafor", "12 oak st apt 4 boston ma"},
		{"joe.rivera@example.com", "Joe Rivera", "77 Lake Drive, Miami FL"},
		{"priya.nair@example.com", "Priya Nair", "5 Maple Street, Suite 200, Chicago IL"},
		{"PRIYA.NAIR@example.com", "Priya Nair", "5 Maple St Ste 200, Chicago IL"},
		{"lena.fischer@example.com", "Lena Fischer", "19 River Road, Portland OR"},
	}
	for i, e := range entries {
		add(`INSERT INTO giveaway_entries (giveaway_id, email, name, address, entered_at)
			VALUES ('G-12',$1,$2,$3,$4) ON CONFLICT DO NOTHING`, e[0], e[1], e[2], day(10).Add(time.Duration(i)*time.Hour))
	}

	if err := a.Pool.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("insert demo rows: %w", err)
	}
	return nil
}
