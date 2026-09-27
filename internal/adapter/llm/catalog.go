package llm

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/davidmm07/pressroom/internal/domain"
	"github.com/davidmm07/pressroom/internal/port"
)

type catalogEntry struct {
	label string
	price domain.Price
}

// defaultCatalog holds list prices in USD per million tokens. Anthropic
// prices come from the published rate card; the others were list prices
// when this table was written and can be overridden at deploy time with
// PRESSROOM_PRICEBOOK_JSON. Open-weights rows are an internal estimate of
// amortised GPU cost, not a vendor price.
var defaultCatalog = map[domain.ModelRef]catalogEntry{
	{Provider: domain.ProviderAnthropic, Name: "claude-opus-5"}:             {"Claude Opus 5", domain.Price{InputPerMTok: 5, OutputPerMTok: 25}},
	{Provider: domain.ProviderAnthropic, Name: "claude-sonnet-5"}:           {"Claude Sonnet 5", domain.Price{InputPerMTok: 2, OutputPerMTok: 10}},
	{Provider: domain.ProviderAnthropic, Name: "claude-haiku-4-5"}:          {"Claude Haiku 4.5", domain.Price{InputPerMTok: 1, OutputPerMTok: 5}},
	{Provider: domain.ProviderAnthropic, Name: "claude-fable-5-1"}:          {"Claude Fable 5.1", domain.Price{InputPerMTok: 10, OutputPerMTok: 50}},
	{Provider: domain.ProviderOpenAI, Name: "gpt-5"}:                        {"GPT-5", domain.Price{InputPerMTok: 1.25, OutputPerMTok: 10}},
	{Provider: domain.ProviderOpenAI, Name: "gpt-5-mini"}:                   {"GPT-5 mini", domain.Price{InputPerMTok: 0.25, OutputPerMTok: 2}},
	{Provider: domain.ProviderXAI, Name: "grok-4"}:                          {"Grok 4", domain.Price{InputPerMTok: 3, OutputPerMTok: 15}},
	{Provider: domain.ProviderXAI, Name: "grok-4-fast"}:                     {"Grok 4 Fast", domain.Price{InputPerMTok: 0.2, OutputPerMTok: 0.5}},
	{Provider: domain.ProviderOpenSource, Name: "qwen/qwen3-32b"}:           {"Qwen3 32B (self-hosted)", domain.Price{InputPerMTok: 0.1, OutputPerMTok: 0.3}},
	{Provider: domain.ProviderOpenSource, Name: "meta-llama/llama-3.3-70b"}: {"Llama 3.3 70B (self-hosted)", domain.Price{InputPerMTok: 0.2, OutputPerMTok: 0.6}},
	{Provider: domain.ProviderSandbox, Name: "planner"}:                     {"Sandbox planner (offline)", domain.Price{}},
}

// unknownPrice is deliberately pessimistic so budgets still bite on a model
// nobody priced yet.
var unknownPrice = domain.Price{InputPerMTok: 10, OutputPerMTok: 50}

// Catalog is the price book and the list of selectable models.
type Catalog struct {
	entries   map[domain.ModelRef]catalogEntry
	available func(domain.Provider) bool
}

var (
	_ port.PriceBook    = (*Catalog)(nil)
	_ port.ModelCatalog = (*Catalog)(nil)
)

// NewCatalog builds the catalog. overridesJSON, when non-empty, maps
// "provider/model" to {"input": x, "output": y} and adds or replaces rows.
func NewCatalog(overridesJSON string, available func(domain.Provider) bool) (*Catalog, error) {
	entries := make(map[domain.ModelRef]catalogEntry, len(defaultCatalog))
	for k, v := range defaultCatalog {
		entries[k] = v
	}
	if overridesJSON != "" {
		var overrides map[string]struct {
			Input  float64 `json:"input"`
			Output float64 `json:"output"`
		}
		if err := json.Unmarshal([]byte(overridesJSON), &overrides); err != nil {
			return nil, fmt.Errorf("price book overrides: %w", err)
		}
		for key, p := range overrides {
			ref, err := domain.ParseModelRef(key)
			if err != nil {
				return nil, fmt.Errorf("price book overrides: %q: %w", key, err)
			}
			label := entries[ref].label
			if label == "" {
				label = ref.Name
			}
			entries[ref] = catalogEntry{label: label, price: domain.Price{InputPerMTok: p.Input, OutputPerMTok: p.Output}}
		}
	}
	return &Catalog{entries: entries, available: available}, nil
}

// Price returns the model's price, or a pessimistic default.
func (c *Catalog) Price(m domain.ModelRef) domain.Price {
	if e, ok := c.entries[m]; ok {
		return e.price
	}
	return unknownPrice
}

// Models lists known models, grouped by provider.
func (c *Catalog) Models() []port.ModelInfo {
	out := make([]port.ModelInfo, 0, len(c.entries))
	for ref, e := range c.entries {
		out = append(out, port.ModelInfo{Ref: ref, Label: e.label, Price: e.price, Available: c.available(ref.Provider)})
	}
	order := map[domain.Provider]int{}
	for i, p := range domain.Providers {
		order[p] = i
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ref.Provider != out[j].Ref.Provider {
			return order[out[i].Ref.Provider] < order[out[j].Ref.Provider]
		}
		return out[i].Price.OutputPerMTok > out[j].Price.OutputPerMTok
	})
	return out
}
