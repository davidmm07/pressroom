package domain

import (
	"fmt"
	"math"
)

// Micros is an amount of US dollars in millionths. LLM calls cost fractions
// of a cent, so cents are too coarse and float64 accumulates rounding drift.
type Micros int64

// MicrosFromUSD converts a dollar amount, rounding to the nearest micro.
func MicrosFromUSD(usd float64) Micros { return Micros(math.Round(usd * 1e6)) }

// USD returns the amount in dollars for presentation.
func (m Micros) USD() float64 { return float64(m) / 1e6 }

// String renders the amount with six decimals, e.g. "0.012500".
func (m Micros) String() string {
	sign := ""
	v := int64(m)
	if v < 0 {
		sign, v = "-", -v
	}
	return fmt.Sprintf("%s%d.%06d", sign, v/1e6, v%1e6)
}

// Usage counts the tokens a model consumed. CachedInputTokens are input
// tokens served from the provider's prompt cache, billed at a discount.
type Usage struct {
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
}

// Add returns the sum of two usages.
func (u Usage) Add(o Usage) Usage {
	return Usage{
		InputTokens:       u.InputTokens + o.InputTokens,
		CachedInputTokens: u.CachedInputTokens + o.CachedInputTokens,
		OutputTokens:      u.OutputTokens + o.OutputTokens,
	}
}

// Price is a model's list price per million tokens.
type Price struct {
	InputPerMTok  float64
	OutputPerMTok float64
	// CachedInputPerMTok defaults to a tenth of the input price when zero,
	// which matches the common prompt-cache discount.
	CachedInputPerMTok float64
}

// Cost prices a usage.
func (p Price) Cost(u Usage) Micros {
	cached := p.CachedInputPerMTok
	if cached == 0 {
		cached = p.InputPerMTok / 10
	}
	usd := float64(u.InputTokens)/1e6*p.InputPerMTok +
		float64(u.CachedInputTokens)/1e6*cached +
		float64(u.OutputTokens)/1e6*p.OutputPerMTok
	return MicrosFromUSD(usd)
}
