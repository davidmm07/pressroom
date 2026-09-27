package tools

// Dependencies are the systems the standard toolbox talks to.
type Dependencies struct {
	Commerce Commerce
	Studio   ImageStudio
	Carrier  Carrier
	Slack    Slack
}

// Standard builds the registry with every print shop tool (Factory).
func Standard(d Dependencies) (*Registry, error) {
	r := NewRegistry()
	all := []Tool{
		InspectArtwork(),
		SendProof(d.Commerce),
		LookupOrder(d.Commerce),
		TrackShipment(d.Carrier),
		DraftReply(d.Commerce),
		IssueRefund(d.Commerce),
		LookupCustomer(d.Commerce),
		CreatePromoCode(d.Commerce),
		QueueEmail(d.Commerce),
		NotifyTeam(d.Slack),
	}
	all = append(all, ImageTools(d.Studio)...)
	if err := r.Register(all...); err != nil {
		return nil, err
	}
	return r, nil
}
