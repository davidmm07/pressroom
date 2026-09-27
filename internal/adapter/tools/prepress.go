package tools

import (
	"context"
	"fmt"
	"math"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

// minimumDPI is the resolution each product needs to print sharply. Fabric
// hides detail, so DTG t-shirts get away with less than stickers.
var minimumDPI = map[string]float64{
	"STICKERS": 300, "LABELS": 300, "MAGNETS": 300, "BUTTONS": 300, "PACKAGING": 300, "TSHIRTS": 150,
}

type InspectInput struct {
	ArtworkURL               string  `json:"artworkUrl"`
	Product                  string  `json:"product"`
	WidthPx                  int     `json:"widthPx"`
	HeightPx                 int     `json:"heightPx"`
	PrintWidthIn             float64 `json:"printWidthIn"`
	PrintHeightIn            float64 `json:"printHeightIn"`
	ColorMode                string  `json:"colorMode"`
	HasTransparentBackground bool    `json:"hasTransparentBackground"`
	IsVector                 bool    `json:"isVector"`
}

// Inspection is the print-readiness report.
type Inspection struct {
	PrintReady       bool     `json:"printReady"`
	EffectiveDPI     int      `json:"effectiveDpi"`
	MinimumDPI       int      `json:"minimumDpi"`
	UpscaleFactor    int      `json:"upscaleFactor,omitempty"`
	Issues           []string `json:"issues"`
	RecommendedTools []string `json:"recommendedTools"`
	// NotNeeded names image tools that would not help this file. Models read
	// it as advice; the sandbox planner treats it as a skip list.
	NotNeeded []string `json:"notNeeded"`
}

// InspectArtwork applies prepress rules to an uploaded file's metadata.
func InspectArtwork() Tool {
	spec := domain.ToolSpec{
		Name: "inspect_artwork",
		Description: "Check an uploaded artwork file for print readiness: effective DPI at the ordered print size, " +
			"aspect ratio, background and color mode. Returns issues and which image tools would fix them.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "artworkUrl":   {"type": "string", "format": "uri"},
		    "product":      {"type": "string", "enum": ["STICKERS","LABELS","MAGNETS","BUTTONS","PACKAGING","TSHIRTS"]},
		    "widthPx":      {"type": "integer", "minimum": 1, "maximum": 30000},
		    "heightPx":     {"type": "integer", "minimum": 1, "maximum": 30000},
		    "printWidthIn": {"type": "number", "exclusiveMinimum": 0, "maximum": 60},
		    "printHeightIn":{"type": "number", "exclusiveMinimum": 0, "maximum": 60},
		    "colorMode":    {"type": "string", "enum": ["RGB","CMYK"]},
		    "hasTransparentBackground": {"type": "boolean"},
		    "isVector":     {"type": "boolean"}
		  },
		  "required": ["artworkUrl","product","widthPx","heightPx","printWidthIn","printHeightIn"],
		  "additionalProperties": false
		}`),
	}
	return Func(spec, func(_ context.Context, in InspectInput) (any, error) { return Inspect(in), nil })
}

// Inspect is the pure rule set, exported for tests and reuse.
func Inspect(in InspectInput) Inspection {
	minDPI := minimumDPI[in.Product]
	r := Inspection{MinimumDPI: int(minDPI), Issues: []string{}, RecommendedTools: []string{}, NotNeeded: []string{}}

	dpi := math.Min(float64(in.WidthPx)/in.PrintWidthIn, float64(in.HeightPx)/in.PrintHeightIn)
	r.EffectiveDPI = int(math.Round(dpi))

	switch {
	case in.IsVector:
		r.EffectiveDPI = 0 // resolution independent
		r.NotNeeded = append(r.NotNeeded, "upscale_image", "vectorize_artwork")
	case dpi >= minDPI:
		r.NotNeeded = append(r.NotNeeded, "upscale_image", "vectorize_artwork")
	case minDPI/dpi <= 4:
		r.UpscaleFactor = int(math.Ceil(minDPI / dpi))
		r.Issues = append(r.Issues, fmt.Sprintf("resolution is %d DPI at %.1fx%.1f in; %s need %d DPI",
			r.EffectiveDPI, in.PrintWidthIn, in.PrintHeightIn, in.Product, int(minDPI)))
		r.RecommendedTools = append(r.RecommendedTools, "upscale_image")
		r.NotNeeded = append(r.NotNeeded, "vectorize_artwork")
	default:
		r.Issues = append(r.Issues, fmt.Sprintf("resolution is %d DPI, too low to upscale cleanly; vectorize it or ask the customer for a larger file", r.EffectiveDPI))
		r.RecommendedTools = append(r.RecommendedTools, "vectorize_artwork")
		r.NotNeeded = append(r.NotNeeded, "upscale_image")
	}

	artAspect := float64(in.WidthPx) / float64(in.HeightPx)
	printAspect := in.PrintWidthIn / in.PrintHeightIn
	if math.Abs(artAspect-printAspect)/printAspect > 0.02 {
		r.Issues = append(r.Issues, fmt.Sprintf("artwork aspect ratio %.2f differs from the print size %.2f; it will be letterboxed or cropped", artAspect, printAspect))
	}

	// Die-cut stickers and shaped magnets follow the artwork's outline, which
	// needs a transparent background.
	if !in.HasTransparentBackground && (in.Product == "STICKERS" || in.Product == "MAGNETS") {
		r.Issues = append(r.Issues, "background is not transparent, so the die line would follow the rectangle")
		r.RecommendedTools = append(r.RecommendedTools, "remove_background")
	} else {
		r.NotNeeded = append(r.NotNeeded, "remove_background")
	}

	if in.ColorMode == "RGB" {
		r.Issues = append(r.Issues, "RGB artwork will be converted to CMYK; saturated blues and greens may shift")
	}
	r.PrintReady = len(r.RecommendedTools) == 0
	return r
}

type sendProofInput struct {
	OrderID    string `json:"orderId"`
	ArtworkURL string `json:"artworkUrl"`
	Note       string `json:"note"`
}

// SendProof publishes the free online proof the customer approves before
// production starts.
func SendProof(c Commerce) Tool {
	spec := domain.ToolSpec{
		Name:        "send_proof",
		Description: "Send the customer a proof of their order using the final artwork, with a short note about any changes made.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "orderId":    {"type": "string", "pattern": "^SM-[0-9]{3,8}$"},
		    "artworkUrl": {"type": "string", "format": "uri"},
		    "note":       {"type": "string", "minLength": 1, "maxLength": 1000}
		  },
		  "required": ["orderId","artworkUrl","note"],
		  "additionalProperties": false
		}`),
	}
	return Func(spec, func(ctx context.Context, in sendProofInput) (any, error) {
		order, err := c.Order(ctx, in.OrderID)
		if err != nil {
			return nil, err
		}
		id, err := c.Record(ctx, "PROOF", idempotencyKey(ctx, "proof"), in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"proofId": id, "orderId": order.ID, "sentTo": order.CustomerEmail, "proofUrl": in.ArtworkURL}, nil
	})
}

// idempotencyKey scopes the call's key to the action, so one call that
// records two things gets two keys.
func idempotencyKey(ctx context.Context, action string) string {
	if info, ok := port.ToolCallFrom(ctx); ok {
		return action + ":" + info.IdempotencyKey()
	}
	return action + ":" + newRef("")
}
