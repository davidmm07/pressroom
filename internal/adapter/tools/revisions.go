package tools

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/davidmm07/pressroom/internal/domain"
)

// Edit is one change a customer asked for on their proof.
type Edit struct {
	Type     string  `json:"type"`
	WidthIn  float64 `json:"widthIn,omitempty"`
	HeightIn float64 `json:"heightIn,omitempty"`
	Shape    string  `json:"shape,omitempty"`
	Border   string  `json:"border,omitempty"`
	Degrees  int     `json:"degrees,omitempty"`
}

// cutShapes lists the shapes each product can be cut to. Shirts and boxes
// have no cut line, and buttons only come round.
var cutShapes = map[string][]string{
	"STICKERS": {"DIE_CUT", "CIRCLE", "SQUARE", "ROUNDED_RECTANGLE", "OVAL"},
	"LABELS":   {"CIRCLE", "SQUARE", "ROUNDED_RECTANGLE", "OVAL"},
	"MAGNETS":  {"DIE_CUT", "CIRCLE", "SQUARE", "ROUNDED_RECTANGLE"},
	"BUTTONS":  {"CIRCLE"},
}

// maxPrintIn is the longest side each product line can print, in inches.
var maxPrintIn = map[string]float64{"STICKERS": 24, "LABELS": 12, "MAGNETS": 24, "BUTTONS": 3, "PACKAGING": 24, "TSHIRTS": 16}

// CheckEdits returns every edit the product cannot take, so the model can
// fix them all in one retry. Exported for tests and reuse.
func CheckEdits(product string, edits []Edit) error {
	var errs []error
	for i, e := range edits {
		switch e.Type {
		case "CHANGE_SHAPE":
			if !slices.Contains(cutShapes[product], e.Shape) {
				errs = append(errs, fmt.Errorf("edits[%d]: %s cannot be cut to %s (options: %v)", i, product, e.Shape, cutShapes[product]))
			}
		case "SET_BORDER":
			if product != "STICKERS" && product != "MAGNETS" {
				errs = append(errs, fmt.Errorf("edits[%d]: only die-cut stickers and magnets have a border", i))
			}
		case "RESIZE":
			if limit := maxPrintIn[product]; max(e.WidthIn, e.HeightIn) > limit {
				errs = append(errs, fmt.Errorf("edits[%d]: %s print at most %.0f in on the longest side", i, product, limit))
			}
		}
	}
	return errors.Join(errs...)
}

// studioStep maps an edit to an image studio operation.
func studioStep(e Edit) (string, map[string]any, string) {
	switch e.Type {
	case "RESIZE":
		return "resize", map[string]any{"widthIn": e.WidthIn, "heightIn": e.HeightIn}, fmt.Sprintf("resized to %gx%g in", e.WidthIn, e.HeightIn)
	case "CHANGE_SHAPE":
		return "shape", map[string]any{"shape": e.Shape}, "cut shape set to " + e.Shape
	case "SET_BORDER":
		return "border", map[string]any{"border": e.Border}, "border set to " + e.Border
	case "ROTATE":
		return "rotate", map[string]any{"degrees": e.Degrees}, fmt.Sprintf("rotated %d degrees", e.Degrees)
	default: // REMOVE_BACKGROUND
		return "remove-background", nil, "background removed"
	}
}

type revisionInput struct {
	ArtworkURL      string `json:"artworkUrl"`
	Product         string `json:"product"`
	Edits           []Edit `json:"edits"`
	DesignerRequest string `json:"designerRequest"`
}

// ApplyProofRevision makes the layout changes a customer asks for on a proof
// with the image studio, and queues anything it cannot do for a designer.
func ApplyProofRevision(c Commerce, studio ImageStudio) Tool {
	return Func(domain.ToolSpec{
		Name: "apply_proof_revision",
		Description: "Apply a customer's requested changes to their proof: resize, cut shape, border, rotation or background removal. " +
			"Changes to text, colors or layout cannot be automated; put them in designerRequest, in the customer's words, for a designer. " +
			"Returns the revised artwork URL.",
		InputSchema: Schema(`{
		  "type": "object",
		  "properties": {
		    "artworkUrl": {"type": "string", "format": "uri"},
		    "product":    {"type": "string", "enum": ["STICKERS","LABELS","MAGNETS","BUTTONS","PACKAGING","TSHIRTS"]},
		    "edits": {
		      "type": "array", "maxItems": 6,
		      "items": {
		        "type": "object",
		        "properties": {
		          "type":     {"type": "string", "enum": ["RESIZE","CHANGE_SHAPE","SET_BORDER","ROTATE","REMOVE_BACKGROUND"]},
		          "widthIn":  {"type": "number", "exclusiveMinimum": 0, "maximum": 60},
		          "heightIn": {"type": "number", "exclusiveMinimum": 0, "maximum": 60},
		          "shape":    {"type": "string", "enum": ["DIE_CUT","CIRCLE","SQUARE","ROUNDED_RECTANGLE","OVAL"]},
		          "border":   {"type": "string", "enum": ["NONE","THIN_WHITE","THICK_WHITE"]},
		          "degrees":  {"type": "integer", "enum": [90,180,270]}
		        },
		        "required": ["type"],
		        "additionalProperties": false,
		        "allOf": [
		          {"if": {"properties": {"type": {"const": "RESIZE"}}},       "then": {"required": ["widthIn","heightIn"]}},
		          {"if": {"properties": {"type": {"const": "CHANGE_SHAPE"}}}, "then": {"required": ["shape"]}},
		          {"if": {"properties": {"type": {"const": "SET_BORDER"}}},   "then": {"required": ["border"]}},
		          {"if": {"properties": {"type": {"const": "ROTATE"}}},       "then": {"required": ["degrees"]}}
		        ]
		      }
		    },
		    "designerRequest": {"type": "string", "minLength": 5, "maxLength": 1000}
		  },
		  "required": ["artworkUrl","product","edits"],
		  "additionalProperties": false
		}`),
	}, func(ctx context.Context, in revisionInput) (any, error) {
		if len(in.Edits) == 0 && in.DesignerRequest == "" {
			return nil, errors.New("nothing to revise: give at least one edit or a designerRequest")
		}
		if err := CheckEdits(in.Product, in.Edits); err != nil {
			return nil, err
		}
		url, applied := in.ArtworkURL, []string{}
		for _, e := range in.Edits {
			op, params, done := studioStep(e)
			res, err := studio.Process(ctx, op, url, params)
			if err != nil {
				return nil, err
			}
			url, applied = res.ArtworkURL, append(applied, done)
		}
		out := map[string]any{"artworkUrl": url, "applied": applied, "needsDesigner": in.DesignerRequest != ""}
		if in.DesignerRequest != "" {
			id, err := c.Record(ctx, "DESIGN_REQUEST", idempotencyKey(ctx, "design"), in)
			if err != nil {
				return nil, err
			}
			out["designRequestId"] = id
		}
		return out, nil
	})
}
