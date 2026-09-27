package tools

import (
	"context"
	"strings"

	"github.com/davidmm07/pressroom/internal/domain"
)

// LookupCustomer summarises a customer's order history for reorder offers.
func LookupCustomer(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:        "lookup_customer",
		Description: "Summarise a customer's order history: lifetime orders and spend, last product and days since the last order.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {"customerEmail": {"type": "string", "format": "email"}},
		  "required": ["customerEmail"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in struct {
		CustomerEmail string `json:"customerEmail"`
	}) (any, error) {
		return c.Customer(ctx, strings.ToLower(in.CustomerEmail))
	})
}

type promoInput struct {
	CustomerEmail string `json:"customerEmail"`
	PercentOff    int    `json:"percentOff"`
	Product       string `json:"product"`
}

// CreatePromoCode issues a single-use discount. The 20% ceiling lives in the
// schema, so no prompt can talk the agent past it.
func CreatePromoCode(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:        "create_promo_code",
		Description: "Create a single-use discount code for one customer and product line, valid for 14 days. At most 20% off.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "customerEmail": {"type": "string", "format": "email"},
		    "percentOff":    {"type": "integer", "minimum": 5, "maximum": 20},
		    "product":       {"type": "string", "enum": ["STICKERS","LABELS","MAGNETS","BUTTONS","PACKAGING","TSHIRTS"]}
		  },
		  "required": ["customerEmail","percentOff","product"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in promoInput) (any, error) {
		id, err := c.Record(ctx, "PROMO", idempotencyKey(ctx, "promo"), in)
		if err != nil {
			return nil, err
		}
		code := "REORDER-" + strings.ToUpper(id[len(id)-6:])
		return map[string]any{"promoCode": code, "percentOff": in.PercentOff, "product": in.Product, "validDays": 14}, nil
	})
}

type emailInput struct {
	CustomerEmail string `json:"customerEmail"`
	Subject       string `json:"subject"`
	Body          string `json:"body"`
}

// QueueEmail schedules a marketing email. Anything that lands in a
// customer's inbox needs approval.
func QueueEmail(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:             "queue_email",
		RequiresApproval: true,
		Description:      "Queue a one-to-one email to a customer. Requires human approval before it is sent.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "customerEmail": {"type": "string", "format": "email"},
		    "subject":       {"type": "string", "minLength": 5, "maxLength": 120},
		    "body":          {"type": "string", "minLength": 20, "maxLength": 4000}
		  },
		  "required": ["customerEmail","subject","body"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in emailInput) (any, error) {
		id, err := c.Record(ctx, "EMAIL", idempotencyKey(ctx, "email"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"emailId": id, "status": "QUEUED"}, nil
	})
}
