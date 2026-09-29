package tools

// Dependencies are the systems the standard toolbox talks to. Each tool
// asks for the narrowest one it needs (Interface Segregation), so one demo
// adapter such as PGCommerce can stand in for several of them.
type Dependencies struct {
	Commerce    Commerce
	Studio      ImageStudio
	Carrier     Carrier
	Slack       Slack
	Vision      Vision
	Marketplace Marketplace
	Factory     Factory
	Reviews     Reviews
	Giveaways   Giveaways
}

// Standard builds the registry with every print shop tool (Factory).
func Standard(d Dependencies) (*Registry, error) {
	r := NewRegistry()
	all := []Tool{
		InspectArtwork(),
		SendProof(d.Commerce),
		ApplyProofRevision(d.Commerce, d.Studio),
		LookupOrder(d.Commerce),
		TrackShipment(d.Carrier),
		DraftReply(d.Commerce),
		CheckReplyStyle(),
		IssueRefund(d.Commerce),
		AssessDamagePhoto(d.Commerce, d.Vision),
		OrderReprint(d.Commerce),
		LookupCustomer(d.Commerce),
		CreatePromoCode(d.Commerce),
		QueueEmail(d.Commerce),
		AuditStore(d.Marketplace),
		ScreenListing(d.Marketplace),
		PublishListing(d.Commerce, d.Marketplace),
		ProductionStatus(d.Factory),
		RerouteJob(d.Commerce, d.Factory),
		CapacityForecast(d.Factory),
		ProposeDeal(d.Commerce),
		FetchReviews(d.Reviews),
		FileDefectReport(d.Commerce),
		ScreenGiveawayEntries(d.Giveaways),
		NotifyTeam(d.Slack),
	}
	all = append(all, ImageTools(d.Studio)...)
	if err := r.Register(all...); err != nil {
		return nil, err
	}
	return r, nil
}
