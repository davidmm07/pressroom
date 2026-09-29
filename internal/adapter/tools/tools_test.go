package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidmm07/pressroom/internal/adapter/tools"
	"github.com/davidmm07/pressroom/internal/port"
)

type fakeCommerce struct {
	mu      sync.Mutex
	records map[string]string // idempotency key -> id
}

func (f *fakeCommerce) Order(_ context.Context, id string) (tools.Order, error) {
	if id != "ORD-1042" {
		return tools.Order{}, tools.ErrNotFound
	}
	return tools.Order{ID: id, CustomerEmail: "ana@example.com", Product: "STICKERS", TotalCents: 4900, PlacedAt: time.Now()}, nil
}

func (f *fakeCommerce) Customer(context.Context, string) (tools.Customer, error) {
	return tools.Customer{}, tools.ErrNotFound
}

func (f *fakeCommerce) Record(_ context.Context, kind, key string, _ any) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.records == nil {
		f.records = map[string]string{}
	}
	if id, ok := f.records[key]; ok {
		return id, nil
	}
	id := strings.ToLower(kind) + "_" + string(rune('a'+len(f.records)))
	f.records[key] = id
	return id, nil
}

func registry(t *testing.T) (*tools.Registry, *fakeCommerce) {
	t.Helper()
	c := &fakeCommerce{}
	r, err := tools.Standard(tools.Dependencies{Commerce: c, Studio: tools.SandboxImageStudio{}, Carrier: tools.SandboxCarrier{}})
	if err != nil {
		t.Fatal(err)
	}
	return r, c
}

func TestRegistryValidatesArgumentsAgainstTheSchema(t *testing.T) {
	r, _ := registry(t)
	tests := []struct {
		tool, args, want string
	}{
		{"lookup_order", `{}`, "missing property 'orderId'"},
		{"lookup_order", `{"orderId":"1042"}`, "/orderId"},
		{"issue_refund", `{"orderId":"ORD-1042","amountCents":900000,"reason":"damaged"}`, "/amountCents"},
		{"create_promo_code", `{"customerEmail":"ana@example.com","percentOff":50,"product":"STICKERS"}`, "/percentOff"},
		{"create_promo_code", `{"customerEmail":"not-an-email","percentOff":10,"product":"STICKERS"}`, "/customerEmail"},
		{"lookup_order", `{"orderId":"ORD-1042","extra":true}`, "additional properties"},
		{"lookup_order", `{"orderId":`, "not valid JSON"},
	}
	for _, tt := range tests {
		_, err := r.Invoke(context.Background(), tt.tool, json.RawMessage(tt.args))
		var inputErr *port.ToolInputError
		if !errors.As(err, &inputErr) || !strings.Contains(inputErr.Error(), tt.want) {
			t.Errorf("%s %s: want ToolInputError mentioning %q, got %v", tt.tool, tt.args, tt.want, err)
		}
	}
}

func TestRegistryCatalog(t *testing.T) {
	r, _ := registry(t)
	names := []string{}
	approval := map[string]bool{}
	for _, s := range r.Catalog() {
		names = append(names, s.Name)
		approval[s.Name] = s.RequiresApproval
	}
	if len(names) != 13 {
		t.Fatalf("catalog = %v", names)
	}
	if !approval["issue_refund"] || !approval["queue_email"] || approval["lookup_order"] {
		t.Fatalf("approval flags wrong: %v", approval)
	}
	if err := r.Register(tools.InspectArtwork()); err == nil {
		t.Fatal("duplicate registration must fail")
	}
}

func TestRefundCannotExceedTheOrderTotal(t *testing.T) {
	r, _ := registry(t)
	_, err := r.Invoke(context.Background(), "issue_refund", json.RawMessage(`{"orderId":"ORD-1042","amountCents":5000,"reason":"misprint"}`))
	if err == nil || !strings.Contains(err.Error(), "exceeds the order total") {
		t.Fatalf("got %v", err)
	}
}

func TestSideEffectsAreIdempotentPerToolCall(t *testing.T) {
	r, c := registry(t)
	ctx := port.WithToolCall(context.Background(), port.ToolCallInfo{RunID: "run-1", CallID: "call-1"})
	args := json.RawMessage(`{"orderId":"ORD-1042","amountCents":1200,"reason":"late delivery"}`)

	first, err := r.Invoke(ctx, "issue_refund", args)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := r.Invoke(ctx, "issue_refund", args) // a retried run replays the call
	if string(first) != string(second) || len(c.records) != 1 {
		t.Fatalf("refund recorded twice: %s vs %s", first, second)
	}
}

func TestInspectArtwork(t *testing.T) {
	base := tools.InspectInput{
		ArtworkURL: "https://cdn.example.com/a.png", Product: "STICKERS",
		WidthPx: 900, HeightPx: 900, PrintWidthIn: 3, PrintHeightIn: 3, ColorMode: "CMYK", HasTransparentBackground: true,
	}
	tests := []struct {
		name      string
		mutate    func(*tools.InspectInput)
		ready     bool
		recommend []string
		factor    int
	}{
		{"300 dpi sticker is ready", func(*tools.InspectInput) {}, true, nil, 0},
		{"150 dpi sticker needs 2x upscale", func(i *tools.InspectInput) { i.PrintWidthIn, i.PrintHeightIn = 6, 6 }, false, []string{"upscale_image"}, 2},
		{"50 dpi needs vectorizing", func(i *tools.InspectInput) { i.PrintWidthIn, i.PrintHeightIn = 18, 18 }, false, []string{"vectorize_artwork"}, 0},
		{"150 dpi is fine on a t-shirt", func(i *tools.InspectInput) { i.Product, i.PrintWidthIn, i.PrintHeightIn = "TSHIRTS", 6, 6 }, true, nil, 0},
		{"opaque die-cut sticker", func(i *tools.InspectInput) { i.HasTransparentBackground = false }, false, []string{"remove_background"}, 0},
		{"vectors ignore resolution", func(i *tools.InspectInput) { i.IsVector, i.WidthPx, i.HeightPx = true, 10, 10 }, true, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base
			tt.mutate(&in)
			got := tools.Inspect(in)
			if got.PrintReady != tt.ready || got.UpscaleFactor != tt.factor || strings.Join(got.RecommendedTools, ",") != strings.Join(tt.recommend, ",") {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestSandboxCarrierIsDeterministic(t *testing.T) {
	c := tools.SandboxCarrier{}
	a, _ := c.Track(context.Background(), "1ZPRESS0001")
	b, _ := c.Track(context.Background(), "1ZPRESS0001")
	if a.Status != b.Status || a.Carrier != b.Carrier {
		t.Fatalf("%+v != %+v", a, b)
	}
}
