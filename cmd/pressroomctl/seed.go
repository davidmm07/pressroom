package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/tools"
	"github.com/davidmm07/pressroom/internal/bootstrap"
	"github.com/davidmm07/pressroom/internal/domain"
)

// seed loads a believable month in the life of the crew. Agents are created
// through the use cases, so the demo data passes the same validation as the
// API. Historical runs are synthetic and written straight to the repository.
func seed(ctx context.Context, a *bootstrap.App, log *slog.Logger) error {
	if _, err := a.Agents.GetBySlug(ctx, "proof-checker"); err == nil {
		log.Info("demo data already loaded; nothing to do")
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	now := time.Now().UTC()
	rng := rand.New(rand.NewPCG(42, 7)) // deterministic demo data

	stalled := findTracking(tools.SandboxCarrier{}, "STALLED")
	inTransit := findTracking(tools.SandboxCarrier{}, "IN_TRANSIT")
	delivered := findTracking(tools.SandboxCarrier{}, "DELIVERED")
	if err := seedOrders(ctx, a, now, stalled, inTransit, delivered); err != nil {
		return fmt.Errorf("seed orders: %w", err)
	}
	if err := seedSystems(ctx, a, now); err != nil {
		return fmt.Errorf("seed systems: %w", err)
	}

	crew := map[string]*domain.Agent{}
	for _, spec := range append(crewSpecs(), newHires()...) {
		agent, err := a.Agents.Create(ctx, spec)
		if err != nil {
			return fmt.Errorf("create %s: %w", spec.Slug, err)
		}
		if spec.Department != domain.DepartmentFinance { // the finance agent stays a draft
			if agent, err = a.Agents.Activate(ctx, agent.ID); err != nil {
				return err
			}
		}
		crew[agent.Slug] = agent
	}

	// A month of history. reorder-nudger works, but reviewers throw most of
	// its emails away: the first evaluation puts it on probation.
	history := []struct {
		slug            string
		runs            int
		success, accept float64
		inTok, outTok   int
	}{
		{"proof-checker", 64, 0.96, 0.9, 7000, 700},
		{"shipping-watch", 38, 0.92, 0.85, 5000, 450},
		{"reorder-nudger", 34, 0.88, 0.35, 4000, 600},
	}
	for _, h := range history {
		agent := crew[h.slug]
		for i := 0; i < h.runs; i++ {
			at := now.Add(-time.Duration(rng.IntN(28*24)+2) * time.Hour)
			if err := seedRun(ctx, a, rng, agent, agent.Model, nil, domain.VariantChampion, at, h.success, h.accept, h.inTok, h.outTok); err != nil {
				return err
			}
		}
	}

	// support-triage is trialling Grok 4 Fast against Claude Opus 5 on 30% of
	// tickets. The challenger is about as good and far cheaper, so
	// concluding the experiment promotes it.
	triage := crew["support-triage"]
	exp, err := domain.NewExperiment(triage, domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4-fast"}, 30,
		"Grok 4 Fast drafts ticket replies as well as Opus 5 at a fraction of the cost", now.AddDate(0, 0, -12))
	if err != nil {
		return err
	}
	if err := a.Store.Experiments.Create(ctx, exp); err != nil {
		return err
	}
	for i := 0; i < 90; i++ {
		at := now.Add(-time.Duration(rng.IntN(11*24)+2) * time.Hour)
		variant := domain.VariantChampion
		success, accept := 0.97, 0.92
		if i%10 < 3 {
			variant, success, accept = domain.VariantChallenger, 0.96, 0.9
		}
		if err := seedRun(ctx, a, rng, triage, exp.ModelFor(variant), exp, variant, at, success, accept, 6000, 550); err != nil {
			return err
		}
	}

	if err := seedOpportunities(ctx, a, crew); err != nil {
		return fmt.Errorf("seed opportunities: %w", err)
	}

	// Live work for the worker: events exactly as Pub/Sub would deliver them.
	events := []struct{ id, typ, data string }{
		{"demo-1", "artwork.uploaded", `{"orderId":"ORD-1048","artworkUrl":"https://cdn.example.com/uploads/ord-1048/fox-logo.png","product":"STICKERS","widthPx":600,"heightPx":600,"printWidthIn":3,"printHeightIn":3,"colorMode":"RGB","hasTransparentBackground":false}`},
		{"demo-2", "artwork.uploaded", `{"orderId":"ORD-1044","artworkUrl":"https://cdn.example.com/uploads/ord-1044/crew-shirt.png","product":"TSHIRTS","widthPx":1800,"heightPx":2400,"printWidthIn":12,"printHeightIn":16,"colorMode":"CMYK","hasTransparentBackground":true}`},
		{"demo-3", "ticket.created", `{"ticketId":"T-5001","orderId":"ORD-1042","customerEmail":"ana.lopez@example.com","message":"Hi! My stickers were supposed to arrive last week and tracking has not moved."}`},
		{"demo-4", "ticket.created", `{"ticketId":"T-5002","orderId":"ORD-1045","customerEmail":"ana.lopez@example.com","message":"Two of the magnets arrived cracked. Could I get a partial refund?","amountCents":1500}`},
		{"demo-5", "customer.reorder_due", `{"customerEmail":"priya.nair@example.com","percentOff":10}`},
	}
	events = append(events, newHireEvents...)
	for _, e := range events {
		if _, err := a.Runs.Dispatch(ctx, e.id, e.typ, json.RawMessage(e.data)); err != nil {
			return fmt.Errorf("dispatch %s: %w", e.typ, err)
		}
	}
	log.Info("demo data loaded", "agents", len(crew), "queued_events", len(events))
	return nil
}

func crewSpecs() []domain.AgentSpec {
	opus := domain.ModelRef{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"}
	return []domain.AgentSpec{
		{
			Slug: "proof-checker", Name: "Proof Checker", Department: domain.DepartmentPrepress, Owner: "prepress-lead@example.com",
			Description: "Pre-flights every uploaded artwork and sends the free online proof.",
			Instructions: `When a customer uploads artwork, make sure it will print well before a prepress technician looks at it.
1. Run inspect_artwork with the file details from the upload event.
2. Apply the fixes it recommends (upscale_image, vectorize_artwork, remove_background), in that order, always on the latest artwork URL.
3. Send the free online proof with send_proof. In the note, tell the customer in one or two friendly sentences what you changed and why, e.g. "We removed the white background so your stickers are cut to shape."
Never change the design itself: colors, text and layout belong to the customer. If the file is too small to fix, say so in the proof note and ask for a larger file.`,
			Model: opus, Tools: []string{"inspect_artwork", "upscale_image", "vectorize_artwork", "remove_background", "send_proof"},
			Triggers: []string{"artwork.uploaded"}, Budget: domain.Budget{MaxSteps: 10, MaxCost: domain.MicrosFromUSD(1)}, MinutesSavedPerRun: 6,
		},
		{
			Slug: "support-triage", Name: "Support Triage", Department: domain.DepartmentCustomerExperience, Owner: "cx-lead@example.com",
			Description: "Researches new tickets and drafts the reply, proposing refunds when warranted.",
			Instructions: `You handle new support tickets. Look up the order before saying anything about it.
- Where is my order: track the shipment and draft a reply with the latest scan and expected delivery.
- Damaged or misprinted items: we reprint or refund, the customer's choice. If they ask for a refund, propose issue_refund for the amount requested, never more than the order total. A person approves every refund.
- Anything you cannot resolve: notify_team with a one-line summary.
Replies are drafts that a support agent sends. Be warm and brief, and sign as "Customer Care".`,
			Model: opus, Tools: []string{"lookup_order", "track_shipment", "draft_reply", "issue_refund", "notify_team"},
			Triggers: []string{"ticket.created"}, Budget: domain.Budget{MaxSteps: 8, MaxCost: domain.MicrosFromUSD(0.75)}, MinutesSavedPerRun: 7,
		},
		{
			Slug: "shipping-watch", Name: "Shipping Watch", Department: domain.DepartmentOperations, Owner: "ops-lead@example.com",
			Description: "Chases parcels with no carrier scan for five days before the customer has to ask.",
			Instructions: `You are called when a parcel has had no carrier scan for five days.
Look up the order and track the shipment. If it is still stalled or has an exception, draft a proactive reply on the ticket from the event (give the latest scan and say what we are doing about it) and notify_team with severity WARNING so operations can open a carrier claim.
If it has since been delivered, do nothing else and say so.`,
			Model: domain.ModelRef{Provider: domain.ProviderOpenAI, Name: "gpt-5-mini"}, Tools: []string{"lookup_order", "track_shipment", "draft_reply", "notify_team"},
			Triggers: []string{"shipment.stalled"}, Budget: domain.Budget{MaxSteps: 6, MaxCost: domain.MicrosFromUSD(0.25)}, MinutesSavedPerRun: 5,
		},
		{
			Slug: "reorder-nudger", Name: "Reorder Nudger", Department: domain.DepartmentMarketing, Owner: "growth-lead@example.com",
			Description: "Emails customers a personal offer when their usual reorder window comes around.",
			Instructions: `Sticker and label customers tend to reorder every 60 to 90 days. When one is due, look up their history.
If their last order was more than 60 days ago, create a promo code for their last product line (10% off, or 15% for customers with three or more orders) and queue a short personal email with the code.
Never email someone who ordered in the last 30 days.`,
			Model: domain.ModelRef{Provider: domain.ProviderOpenSource, Name: "qwen/qwen3-32b"}, Tools: []string{"lookup_customer", "create_promo_code", "queue_email"},
			Triggers: []string{"customer.reorder_due"}, Budget: domain.Budget{MaxSteps: 5, MaxCost: domain.MicrosFromUSD(0.1)}, MinutesSavedPerRun: 4,
		},
		{
			Slug: "payout-reconciler", Name: "Payout Reconciler", Department: domain.DepartmentFinance, Owner: "finance-lead@example.com",
			Description:  "Matches processor payouts to orders. Waiting for finance to review its instructions.",
			Instructions: "Match each payout from the payment processor to its orders and flag any mismatch over one dollar to the finance channel with notify_team.",
			Model:        domain.ModelRef{Provider: domain.ProviderXAI, Name: "grok-4"}, Tools: []string{"lookup_order", "notify_team"},
			Budget: domain.Budget{MaxSteps: 12, MaxCost: domain.MicrosFromUSD(0.5)}, MinutesSavedPerRun: 20,
		},
	}
}

// seedRun writes one finished historical run.
func seedRun(ctx context.Context, a *bootstrap.App, rng *rand.Rand, agent *domain.Agent, model domain.ModelRef,
	exp *domain.Experiment, variant domain.Variant, at time.Time, success, accept float64, inTok, outTok int,
) error {
	params := domain.NewRunParams{Input: json.RawMessage(`{"source":"history"}`), Trigger: domain.TriggerEvent, Model: model, Variant: variant}
	if exp != nil {
		params.ExperimentID = exp.ID
	}
	run, err := domain.NewRun(agent, params, at)
	if err != nil {
		return err
	}
	jitter := 0.7 + rng.Float64()*0.6
	run.Usage = domain.Usage{InputTokens: int(float64(inTok) * jitter), CachedInputTokens: inTok / 2, OutputTokens: int(float64(outTok) * jitter)}
	run.Cost = a.Catalog.Price(model).Cost(run.Usage)
	run.Turns = 2 + rng.IntN(4)
	started := at.Add(time.Second)
	finished := started.Add(time.Duration(8+rng.IntN(40)) * time.Second)
	run.StartedAt, run.FinishedAt = &started, &finished
	if rng.Float64() < success {
		run.Status, run.Output = domain.RunSucceeded, "Completed (historical demo run)."
		if rng.Float64() < 0.8 { // most runs get reviewed
			verdict := domain.VerdictRejected
			switch r := rng.Float64(); {
			case r < accept:
				verdict = domain.VerdictAccepted
			case r < accept+0.08:
				verdict = domain.VerdictEdited
			}
			run.Review = &domain.Review{Verdict: verdict, Reviewer: agent.Owner, At: finished.Add(time.Hour)}
		}
	} else {
		run.Status, run.FailureReason = domain.RunFailed, "tool error: carrier API timed out (historical demo run)"
	}
	return a.Store.Runs.Create(ctx, run)
}

func seedOrders(ctx context.Context, a *bootstrap.App, now time.Time, stalled, inTransit, delivered string) error {
	day := func(n int) time.Time { return now.AddDate(0, 0, -n) }
	type order struct {
		id, email, name, product string
		qty, cents               int
		status, proof            string
		carrier, tracking        *string
		placed                   time.Time
		shipped                  *time.Time
	}
	str := func(s string) *string { return &s }
	tm := func(t time.Time) *time.Time { return &t }
	orders := []order{
		{"ORD-1042", "ana.lopez@example.com", "Ana Lopez", "STICKERS", 250, 8900, "SHIPPED", "APPROVED", str("UPS"), str(stalled), day(12), tm(day(9))},
		{"ORD-1043", "mike.chen@example.com", "Mike Chen", "LABELS", 1000, 16400, "IN_PRODUCTION", "APPROVED", nil, nil, day(3), nil},
		{"ORD-1044", "sara.okafor@example.com", "Sara Okafor", "TSHIRTS", 48, 61200, "PROOF_PENDING", "NOT_SENT", nil, nil, day(1), nil},
		{"ORD-1045", "ana.lopez@example.com", "Ana Lopez", "MAGNETS", 100, 7400, "DELIVERED", "APPROVED", str("USPS"), str(delivered), day(20), tm(day(17))},
		{"ORD-1046", "joe.rivera@example.com", "Joe Rivera", "BUTTONS", 200, 5800, "SHIPPED", "APPROVED", str("DHL"), str(inTransit), day(6), tm(day(3))},
		{"ORD-1047", "sara.okafor@example.com", "Sara Okafor", "PACKAGING", 500, 124000, "PROOF_PENDING", "NOT_SENT", nil, nil, day(2), nil},
		{"ORD-1048", "joe.rivera@example.com", "Joe Rivera", "STICKERS", 300, 9900, "PROOF_PENDING", "NOT_SENT", nil, nil, day(0), nil},
		{"ORD-1049", "mia.santos@example.com", "Mia Santos", "STICKERS", 200, 7900, "PROOF_PENDING", "CHANGES_REQUESTED", nil, nil, day(1), nil},
		{"ORD-1050", "tom.becker@example.com", "Tom Becker", "STICKERS", 500, 14500, "DELIVERED", "APPROVED", str("UPS"), str("1ZPR1050"), day(9), tm(day(6))},
		{"ORD-0981", "priya.nair@example.com", "Priya Nair", "STICKERS", 500, 14500, "DELIVERED", "APPROVED", str("UPS"), str("1ZPR0981"), day(210), tm(day(206))},
		{"ORD-0990", "priya.nair@example.com", "Priya Nair", "STICKERS", 500, 14500, "DELIVERED", "APPROVED", str("UPS"), str("1ZPR0990"), day(150), tm(day(146))},
		{"ORD-1001", "priya.nair@example.com", "Priya Nair", "LABELS", 1000, 18900, "DELIVERED", "APPROVED", str("UPS"), str("1ZPR1001"), day(84), tm(day(80))},
	}
	for _, o := range orders {
		_, err := a.Pool.Exec(ctx, `INSERT INTO storefront_orders
			(id, customer_email, customer_name, product, quantity, status, total_cents, carrier, tracking_number, proof_status, placed_at, shipped_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (id) DO NOTHING`,
			o.id, o.email, o.name, o.product, o.qty, o.status, o.cents, o.carrier, o.tracking, o.proof, o.placed, o.shipped)
		if err != nil {
			return err
		}
	}
	return nil
}

func seedOpportunities(ctx context.Context, a *bootstrap.App, crew map[string]*domain.Agent) error {
	items := []struct {
		in     domain.OpportunityInput
		status []domain.OpportunityStatus
		agent  string
	}{
		{domain.OpportunityInput{Title: "Answer where-is-my-order tickets", Problem: "About half of support tickets ask where an order is. Agents copy tracking from the carrier site by hand.",
			Department: domain.DepartmentCustomerExperience, SubmittedBy: "cx-lead@example.com", WeeklyVolume: 600, MinutesPerTask: 4,
			DataSensitivity: domain.LevelMedium, ErrorCost: domain.LevelLow}, []domain.OpportunityStatus{domain.OpportunityApproved, domain.OpportunityShipped}, "support-triage"},
		{domain.OpportunityInput{Title: "Pre-flight uploaded artwork", Problem: "Technicians open every upload to check resolution and background before the proof goes out.",
			Department: domain.DepartmentPrepress, SubmittedBy: "prepress-lead@example.com", WeeklyVolume: 900, MinutesPerTask: 3,
			DataSensitivity: domain.LevelLow, ErrorCost: domain.LevelMedium}, []domain.OpportunityStatus{domain.OpportunityApproved, domain.OpportunityShipped}, "proof-checker"},
		{domain.OpportunityInput{Title: "Chase stalled international parcels", Problem: "Parcels stuck at customs are only noticed when the customer complains.",
			Department: domain.DepartmentOperations, SubmittedBy: "ops-lead@example.com", WeeklyVolume: 120, MinutesPerTask: 6,
			DataSensitivity: domain.LevelMedium, ErrorCost: domain.LevelMedium}, []domain.OpportunityStatus{domain.OpportunityApproved, domain.OpportunityShipped}, "shipping-watch"},
		{domain.OpportunityInput{Title: "Reconcile processor payouts", Problem: "Finance matches payouts to orders in a spreadsheet every week.",
			Department: domain.DepartmentFinance, SubmittedBy: "finance-lead@example.com", WeeklyVolume: 40, MinutesPerTask: 20,
			DataSensitivity: domain.LevelHigh, ErrorCost: domain.LevelHigh}, []domain.OpportunityStatus{domain.OpportunityApproved}, ""},
		{domain.OpportunityInput{Title: "Review damage claim photos", Problem: "Every claim photo is opened by hand and compared with the proof before we offer a reprint or refund.",
			Department: domain.DepartmentCustomerExperience, SubmittedBy: "cx-lead@example.com", WeeklyVolume: 150, MinutesPerTask: 8,
			DataSensitivity: domain.LevelMedium, ErrorCost: domain.LevelMedium}, []domain.OpportunityStatus{domain.OpportunityApproved, domain.OpportunityShipped}, "claim-assessor"},
		{domain.OpportunityInput{Title: "Get new stores to their first sale", Problem: "Most marketplace stores never sell, and nobody has time to tell sellers what to fix.",
			Department: domain.DepartmentMarketplace, SubmittedBy: "marketplace-lead@example.com", WeeklyVolume: 400, MinutesPerTask: 15,
			DataSensitivity: domain.LevelLow, ErrorCost: domain.LevelLow}, []domain.OpportunityStatus{domain.OpportunityApproved, domain.OpportunityShipped}, "store-coach"},
		{domain.OpportunityInput{Title: "Catch jobs that will miss their ship date", Problem: "Late jobs are found at the packing table, when it is too late to move them to a free press.",
			Department: domain.DepartmentManufacturing, SubmittedBy: "manufacturing-lead@example.com", WeeklyVolume: 60, MinutesPerTask: 12,
			DataSensitivity: domain.LevelLow, ErrorCost: domain.LevelMedium}, []domain.OpportunityStatus{domain.OpportunityApproved, domain.OpportunityShipped}, "production-watch"},
		{domain.OpportunityInput{Title: "Write listings for new sticker shapes", Problem: "Every new die-cut shape needs a product description and alt text.",
			Department: domain.DepartmentMarketing, SubmittedBy: "growth-lead@example.com", WeeklyVolume: 15, MinutesPerTask: 30,
			DataSensitivity: domain.LevelLow, ErrorCost: domain.LevelLow}, nil, ""},
	}
	for _, it := range items {
		o, err := a.Opportunities.Submit(ctx, it.in)
		if err != nil {
			return err
		}
		for _, st := range it.status {
			var agentID domain.ID
			if st == domain.OpportunityShipped {
				agentID = crew[it.agent].ID
			}
			if o, err = a.Opportunities.Move(ctx, o.ID, st, agentID); err != nil {
				return err
			}
		}
	}
	return nil
}

// findTracking returns a tracking number the sandbox carrier reports with
// the given status, so the demo tells the same story on every machine.
func findTracking(c tools.SandboxCarrier, status string) string {
	for i := 1000; ; i++ {
		tn := fmt.Sprintf("1ZPRESS%04d", i)
		if s, _ := c.Track(context.Background(), tn); s.Status == status {
			return tn
		}
	}
}
