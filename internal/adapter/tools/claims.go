package tools

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Damage is what a vision model sees in a customer's photo of their order.
type Damage struct {
	Defect      string  `json:"defect"`
	Confidence  float64 `json:"confidence"` // 0 to 1
	Observation string  `json:"observation"`
}

// defectCause separates faults made on the factory floor, which
// manufacturing needs to hear about, from damage in transit.
var defectCause = map[string]string{
	"MISPRINT": "PRODUCTION", "COLOR_SHIFT": "PRODUCTION", "MISCUT": "PRODUCTION", "PEELING": "PRODUCTION",
	"CRACKED": "TRANSIT", "BENT": "TRANSIT", "WATER_DAMAGE": "TRANSIT",
	"NONE_VISIBLE": "NONE",
}

// Vision looks at photos. Production sends the photo and the approved proof
// to a multimodal model; the sandbox reads the defect from the file name.
type Vision interface {
	InspectDamage(ctx context.Context, photoURL, product string) (Damage, error)
}

// SandboxVision finds the defect named in the photo's file name, such as
// cracked-magnets.jpg, so demo and test photos say what they show. A file
// name with "blurry" in it gets a low-confidence answer.
type SandboxVision struct{}

func (SandboxVision) InspectDamage(_ context.Context, photoURL, product string) (Damage, error) {
	u, err := url.Parse(photoURL)
	if err != nil {
		return Damage{}, err
	}
	name := strings.ToLower(path.Base(u.Path))
	d := Damage{Defect: "NONE_VISIBLE", Confidence: 0.9, Observation: "no damage visible on the " + strings.ToLower(product)}
	for _, k := range []struct{ word, defect string }{
		{"misprint", "MISPRINT"}, {"color", "COLOR_SHIFT"}, {"miscut", "MISCUT"}, {"peel", "PEELING"},
		{"crack", "CRACKED"}, {"bent", "BENT"}, {"water", "WATER_DAMAGE"},
	} {
		if strings.Contains(name, k.word) {
			d.Defect, d.Confidence = k.defect, 0.93
			d.Observation = strings.ToLower(strings.ReplaceAll(k.defect, "_", " ")) + " visible on the " + strings.ToLower(product)
			break
		}
	}
	if strings.Contains(name, "blurry") {
		d.Confidence, d.Observation = 0.4, "the photo is too blurry to judge"
	}
	return d, nil
}

// ClaimInput is a customer's damage claim.
type ClaimInput struct {
	OrderID         string `json:"orderId"`
	PhotoURL        string `json:"photoUrl"`
	ClaimedUnits    int    `json:"claimedUnits"`
	PreferredRemedy string `json:"preferredRemedy"`
}

// Assessment is the remedy the claims policy proposes.
type Assessment struct {
	OrderID      string   `json:"orderId"`
	CustomerName string   `json:"customerName"`
	Defect       string   `json:"defect"`
	Cause        string   `json:"cause"`
	Confidence   float64  `json:"confidence"`
	Observation  string   `json:"observation"`
	Remedy       string   `json:"remedy"` // REPRINT, REFUND or HUMAN_REVIEW
	ReprintUnits int      `json:"reprintUnits,omitempty"`
	AmountCents  int      `json:"amountCents,omitempty"`
	Explanation  string   `json:"explanation"`
	NotNeeded    []string `json:"notNeeded"`
}

// minClaimConfidence is how sure the vision model must be before an agent
// proposes a remedy without a person looking at the photo first.
const minClaimConfidence = 0.7

// AssessClaim applies the claims policy: the photo is the evidence, nothing
// has to be sent back, and the customer chooses a reprint or a refund of the
// affected units. Exported for tests and reuse.
func AssessClaim(o Order, d Damage, in ClaimInput) Assessment {
	a := Assessment{
		OrderID: o.ID, CustomerName: o.CustomerName, Defect: d.Defect, Cause: defectCause[d.Defect],
		Confidence: d.Confidence, Observation: d.Observation,
	}
	units := min(in.ClaimedUnits, o.Quantity)
	humanReview := func(why string) Assessment {
		a.Remedy, a.Explanation = "HUMAN_REVIEW", why
		a.NotNeeded = []string{"order_reprint", "issue_refund"}
		return a
	}
	switch {
	case d.Defect == "NONE_VISIBLE":
		return humanReview("The photo shows no damage. A person should look at it and may ask for another photo.")
	case d.Confidence < minClaimConfidence:
		return humanReview(fmt.Sprintf("Only %.0f%% sure what the photo shows. A person should look at it.", d.Confidence*100))
	case o.Quantity <= 0:
		return humanReview("The order has no quantity to price the affected units against.")
	case in.PreferredRemedy == "REFUND":
		amount := (o.TotalCents*units + o.Quantity - 1) / o.Quantity // round up in the customer's favour
		if amount > MaxAgentRefundCents {
			return humanReview(fmt.Sprintf("A refund of %d cents is more than an agent may propose.", amount))
		}
		a.Remedy, a.AmountCents = "REFUND", amount
		a.Explanation = fmt.Sprintf("Refund %d of %d units, as the customer asked.", units, o.Quantity)
		a.NotNeeded = []string{"order_reprint"}
	default:
		a.Remedy, a.ReprintUnits = "REPRINT", units
		a.Explanation = fmt.Sprintf("Reprint %d of %d units and ship them free.", units, o.Quantity)
		a.NotNeeded = []string{"issue_refund"}
	}
	// Transit damage is the carrier's problem. Production faults also go to
	// the team, so manufacturing hears about them the same day.
	if a.Cause == "TRANSIT" {
		a.NotNeeded = append(a.NotNeeded, "notify_team")
	}
	return a
}

// AssessDamagePhoto reviews a claim photo against the order.
func AssessDamagePhoto(c Commerce, v Vision) Tool {
	return Func(domain.ToolSpec{
		Name: "assess_damage_photo",
		Description: "Look at the customer's photo of a damaged or misprinted order and propose a remedy under the claims policy: " +
			"reprint or refund the affected units, or send it to a person when the photo is unclear. Nothing has to be sent back.",
		InputSchema: Schema(fmt.Sprintf(`{
		  "type": "object",
		  "properties": {
		    "orderId":         %s,
		    "photoUrl":        {"type": "string", "format": "uri"},
		    "claimedUnits":    {"type": "integer", "minimum": 1, "maximum": 100000, "description": "How many items the customer says are affected"},
		    "preferredRemedy": {"type": "string", "enum": ["REPRINT","REFUND"], "default": "REPRINT"}
		  },
		  "required": ["orderId","photoUrl","claimedUnits"],
		  "additionalProperties": false
		}`, orderIDSchema)),
	}, func(ctx context.Context, in ClaimInput) (any, error) {
		order, err := c.Order(ctx, in.OrderID)
		if err != nil {
			return nil, err
		}
		damage, err := v.InspectDamage(ctx, in.PhotoURL, order.Product)
		if err != nil {
			return nil, err
		}
		return AssessClaim(order, damage, in), nil
	})
}

type reprintInput struct {
	OrderID      string `json:"orderId"`
	ReprintUnits int    `json:"reprintUnits"`
	Reason       string `json:"reason"`
}

// OrderReprint sends replacement items through production at no charge. It
// spends material and press time, so a person approves it.
func OrderReprint(c Commerce) Tool {
	return Func(domain.ToolSpec{
		Name:             "order_reprint",
		RequiresApproval: true,
		Description:      "Reprint the affected units of an order at no charge and ship them free. Requires human approval.",
		InputSchema: Schema(fmt.Sprintf(`{
		  "type": "object",
		  "properties": {
		    "orderId":      %s,
		    "reprintUnits": {"type": "integer", "minimum": 1, "maximum": 100000},
		    "reason":       {"type": "string", "minLength": 5, "maxLength": 500}
		  },
		  "required": ["orderId","reprintUnits","reason"],
		  "additionalProperties": false
		}`, orderIDSchema)),
	}, func(ctx context.Context, in reprintInput) (any, error) {
		order, err := c.Order(ctx, in.OrderID)
		if err != nil {
			return nil, err
		}
		if in.ReprintUnits > order.Quantity {
			return nil, fmt.Errorf("reprint of %d units exceeds the %d ordered", in.ReprintUnits, order.Quantity)
		}
		id, err := c.Record(ctx, "REPRINT", idempotencyKey(ctx, "reprint"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"reprintId": id, "orderId": order.ID, "reprintUnits": in.ReprintUnits, "status": "QUEUED"}, nil
	})
}
