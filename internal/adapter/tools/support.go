package tools

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
)

type orderInput struct {
	OrderID string `json:"orderId"`
}

const orderIDSchema = `{"type": "string", "pattern": "^ORD-[0-9]{3,8}$", "description": "Order number, e.g. ORD-1042"}`

// LookupOrder reads an order from the storefront.
func LookupOrder(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:        "lookup_order",
		Description: "Look up an order by number: product, quantity, status, proof status, total and shipping details.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"orderId": ` + orderIDSchema + `},
		  "required": ["orderId"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in orderInput) (any, error) {
		return c.Order(ctx, in.OrderID)
	})
}

// Shipment is the carrier's tracking status.
type Shipment struct {
	TrackingNumber    string `json:"trackingNumber"`
	Carrier           string `json:"carrier"`
	Status            string `json:"shipmentStatus"` // IN_TRANSIT, DELIVERED, EXCEPTION, STALLED
	LastScan          string `json:"lastScan"`
	DaysSinceLastScan int    `json:"daysSinceLastScan"`
	EstimatedDelivery string `json:"estimatedDelivery,omitempty"`
}

// Carrier tracks parcels. Production would call each carrier's API (or an
// aggregator); the sandbox returns stable fake statuses.
type Carrier interface {
	Track(ctx context.Context, trackingNumber string) (Shipment, error)
}

// SandboxCarrier derives a status from a hash of the tracking number so the
// same parcel always tells the same story.
type SandboxCarrier struct{ Now func() time.Time }

func (c SandboxCarrier) Track(_ context.Context, tn string) (Shipment, error) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(tn))
	n := h.Sum32()
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	s := Shipment{TrackingNumber: tn, Carrier: []string{"UPS", "USPS", "DHL"}[n%3]}
	switch n % 10 {
	case 0, 1, 2, 3, 4, 5:
		s.Status, s.LastScan, s.DaysSinceLastScan = "IN_TRANSIT", "Departed regional facility", 1
		s.EstimatedDelivery = now.AddDate(0, 0, 2).Format("2006-01-02")
	case 6, 7:
		s.Status, s.LastScan = "DELIVERED", "Delivered, left at front door"
	case 8:
		s.Status, s.LastScan, s.DaysSinceLastScan = "EXCEPTION", "Address could not be verified", 2
	default:
		s.Status, s.LastScan, s.DaysSinceLastScan = "STALLED", "Arrived at international hub", 6
	}
	return s, nil
}

// TrackShipment asks the carrier where a parcel is.
func TrackShipment(carrier Carrier) Tool {
	return Func(domain.ToolSpec{
		Name:        "track_shipment",
		Description: "Get the latest carrier scan for a tracking number. STALLED means no scan for 5+ days.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"trackingNumber": {"type": "string", "minLength": 6, "maxLength": 40}},
		  "required": ["trackingNumber"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		TrackingNumber string `json:"trackingNumber"`
	}) (any, error) {
		return carrier.Track(ctx, in.TrackingNumber)
	})
}

type draftInput struct {
	TicketID string `json:"ticketId"`
	Body     string `json:"body"`
}

// DraftReply saves a reply in the helpdesk for an agent to review and send.
// Drafting, not sending, keeps a human between the model and the customer.
func DraftReply(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:        "draft_reply",
		Description: "Save a draft reply on a support ticket. A support agent reviews drafts before they are sent.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "ticketId": {"type": "string", "pattern": "^T-[0-9]+$"},
		    "body":     {"type": "string", "minLength": 20, "maxLength": 4000}
		  },
		  "required": ["ticketId","body"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in draftInput) (any, error) {
		id, err := c.Record(ctx, "DRAFT_REPLY", idempotencyKey(ctx, "draft"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"draftId": id, "ticketId": in.TicketID, "status": "DRAFTED"}, nil
	})
}

type refundInput struct {
	OrderID     string `json:"orderId"`
	AmountCents int    `json:"amountCents"`
	Reason      string `json:"reason"`
}

// MaxAgentRefundCents caps what an agent may even propose. Larger refunds go
// through the finance team's own process.
const MaxAgentRefundCents = 50_000

// IssueRefund refunds part or all of an order. It requires approval: the run
// pauses until a person accepts or denies the exact arguments.
func IssueRefund(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:             "issue_refund",
		RequiresApproval: true,
		Description:      "Refund part or all of an order to the original payment method. Requires human approval.",
		InputSchema: Schema(fmt.Sprintf(`{
		  "type": "object",
		  "properties": {
		    "orderId":     %s,
		    "amountCents": {"type": "integer", "minimum": 1, "maximum": %d},
		    "reason":      {"type": "string", "minLength": 5, "maxLength": 500}
		  },
		  "required": ["orderId","amountCents","reason"],
		  "additionalProperties": false
		}`, orderIDSchema, MaxAgentRefundCents)),
	}, func(ctx context.Context, in refundInput) (any, error) {
		order, err := c.Order(ctx, in.OrderID)
		if err != nil {
			return nil, err
		}
		// Business rule the schema cannot express: never refund more than was paid.
		if in.AmountCents > order.TotalCents {
			return nil, fmt.Errorf("refund of %d cents exceeds the order total of %d cents", in.AmountCents, order.TotalCents)
		}
		id, err := c.Record(ctx, "REFUND", idempotencyKey(ctx, "refund"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"refundId": id, "orderId": order.ID, "amountCents": in.AmountCents, "status": "SUBMITTED"}, nil
	})
}
