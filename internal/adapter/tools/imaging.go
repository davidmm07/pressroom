package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/davidmm07/pressroom/internal/domain"
)

// ImageResult is what the image studio returns for every operation.
type ImageResult struct {
	ArtworkURL string `json:"artworkUrl"`
	Operation  string `json:"operation"`
	WidthPx    int    `json:"widthPx,omitempty"`
	HeightPx   int    `json:"heightPx,omitempty"`
}

// ImageStudio is the company's image service: the same background removal,
// upscaling and vectorization customers use on the website.
type ImageStudio interface {
	Process(ctx context.Context, operation, artworkURL string, params map[string]any) (ImageResult, error)
}

// HTTPImageStudio calls the image service's REST API.
type HTTPImageStudio struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func (s HTTPImageStudio) Process(ctx context.Context, operation, artworkURL string, params map[string]any) (ImageResult, error) {
	body := map[string]any{"artworkUrl": artworkURL}
	for k, v := range params {
		body[k] = v
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.BaseURL, "/")+"/v1/"+operation, bytes.NewReader(payload))
	if err != nil {
		return ImageResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.Token)
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 25 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ImageResult{}, fmt.Errorf("image studio %s: %w", operation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return ImageResult{}, fmt.Errorf("image studio %s: %s: %s", operation, resp.Status, bytes.TrimSpace(msg))
	}
	var out ImageResult
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// SandboxImageStudio pretends to process images by deriving a new URL, so
// demos show the full flow without the real service.
type SandboxImageStudio struct{}

func (SandboxImageStudio) Process(_ context.Context, operation, artworkURL string, params map[string]any) (ImageResult, error) {
	u, err := url.Parse(artworkURL)
	if err != nil {
		return ImageResult{}, err
	}
	q := u.Query()
	q.Set("op", operation)
	if f, ok := params["factor"]; ok {
		q.Set("factor", fmt.Sprint(f))
	}
	u.RawQuery = q.Encode()
	return ImageResult{ArtworkURL: u.String(), Operation: operation}, nil
}

type imageInput struct {
	ArtworkURL string `json:"artworkUrl"`
	Factor     int    `json:"factor"`
}

// ImageTools exposes the studio's three operations as separate tools, so
// each gets its own precise description and schema.
func ImageTools(studio ImageStudio) []Tool {
	artworkOnly := `{
	  "type": "object",
	  "properties": {"artworkUrl": {"type": "string", "format": "uri"}},
	  "required": ["artworkUrl"],
	  "additionalProperties": false
	}`
	return []Tool{
		Func(domain.ToolSpec{
			Name:        "remove_background",
			Description: "Remove the background of an artwork so die-cut stickers and magnets follow its outline. Returns the new artwork URL.",
			InputSchema: Schema(artworkOnly),
		}, func(ctx context.Context, in imageInput) (any, error) {
			return studio.Process(ctx, "remove-background", in.ArtworkURL, nil)
		}),
		Func(domain.ToolSpec{
			Name:        "upscale_image",
			Description: "Enlarge a raster artwork with AI upscaling by a factor of 2 to 4 to reach print resolution. Returns the new artwork URL.",
			InputSchema: Schema(`{
			  "type": "object",
			  "properties": {
			    "artworkUrl": {"type": "string", "format": "uri"},
			    "factor":     {"type": "integer", "minimum": 2, "maximum": 4}
			  },
			  "required": ["artworkUrl","factor"],
			  "additionalProperties": false
			}`),
		}, func(ctx context.Context, in imageInput) (any, error) {
			return studio.Process(ctx, "upscale", in.ArtworkURL, map[string]any{"factor": in.Factor})
		}),
		Func(domain.ToolSpec{
			Name:        "vectorize_artwork",
			Description: "Convert a logo or flat illustration to vector graphics so it prints sharp at any size. Not suitable for photos.",
			InputSchema: Schema(artworkOnly),
		}, func(ctx context.Context, in imageInput) (any, error) {
			return studio.Process(ctx, "vectorize", in.ArtworkURL, nil)
		}),
	}
}
