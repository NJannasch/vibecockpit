package costs

import (
	"strings"
	"time"

	"vibecockpit/internal/provider"
)

// Price per million tokens (USD)
type ModelPrice struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

// Pricing as of April 2026. Models not listed get a rough estimate.
var pricing = map[string]ModelPrice{
	// Anthropic
	"claude-opus-4-7":      {Input: 15.0, Output: 75.0, CacheRead: 1.5, CacheWrite: 18.75},
	"claude-opus-4-6":      {Input: 15.0, Output: 75.0, CacheRead: 1.5, CacheWrite: 18.75},
	"claude-opus-4-6[1m]":  {Input: 15.0, Output: 75.0, CacheRead: 1.5, CacheWrite: 18.75},
	"claude-opus-4-5":      {Input: 15.0, Output: 75.0, CacheRead: 1.5, CacheWrite: 18.75},
	"claude-sonnet-4-6":    {Input: 3.0, Output: 15.0, CacheRead: 0.3, CacheWrite: 3.75},
	"claude-sonnet-4-5":    {Input: 3.0, Output: 15.0, CacheRead: 0.3, CacheWrite: 3.75},
	"claude-haiku-4-5":     {Input: 0.8, Output: 4.0, CacheRead: 0.08, CacheWrite: 1.0},

	// OpenAI
	"gpt-5.4":              {Input: 2.5, Output: 10.0},
	"gpt-5.3-codex":        {Input: 2.5, Output: 10.0},
	"gpt-5.2":              {Input: 2.5, Output: 10.0},
	"gpt-5-mini":           {Input: 0.15, Output: 0.6},
	"codex-mini-latest":    {Input: 1.5, Output: 6.0, CacheRead: 0.375},
	"o3-pro":               {Input: 20.0, Output: 80.0},

	// Google
	"gemini-3-pro":         {Input: 1.25, Output: 5.0},
	"gemini-3-pro-preview": {Input: 1.25, Output: 5.0},
	"gemini-3-flash":       {Input: 0.075, Output: 0.3},
	"gemini-3-flash-preview": {Input: 0.075, Output: 0.3},
	"gemini-2.5-pro":       {Input: 1.25, Output: 5.0},
}

// Subscription plans for reference
type PlanInfo struct {
	Name     string  `json:"name"`
	Provider string  `json:"provider"`
	Price    float64 `json:"priceUsd"`
	Period   string  `json:"period"`
	Note     string  `json:"note"`
}

var Plans = []PlanInfo{
	{Name: "Claude Code Max 5x", Provider: "Anthropic", Price: 100, Period: "month", Note: "5x usage vs Pro"},
	{Name: "Claude Code Max 20x", Provider: "Anthropic", Price: 200, Period: "month", Note: "20x usage vs Pro"},
	{Name: "Claude Pro", Provider: "Anthropic", Price: 20, Period: "month", Note: "Standard limits"},
	{Name: "Codex Pro", Provider: "OpenAI", Price: 200, Period: "month", Note: "Unlimited Codex CLI"},
	{Name: "Copilot Individual", Provider: "GitHub", Price: 10, Period: "month", Note: "Flat rate"},
	{Name: "Gemini Advanced", Provider: "Google", Price: 20, Period: "month", Note: "Includes CLI access"},
}

// GetPricingTable returns the full pricing data for display.
func GetPricingTable() map[string]ModelPrice {
	result := make(map[string]ModelPrice, len(pricing))
	for k, v := range pricing {
		result[k] = v
	}
	return result
}

// Default for unknown models — rough midrange estimate
var defaultPrice = ModelPrice{Input: 3.0, Output: 15.0}

func lookupPrice(model string) ModelPrice {
	if p, ok := pricing[model]; ok {
		return p
	}
	// Try partial match (strip provider prefix like "openai/gpt-5.4")
	if idx := strings.LastIndex(model, "/"); idx >= 0 {
		if p, ok := pricing[model[idx+1:]]; ok {
			return p
		}
	}
	// Try prefix match (handle version suffixes)
	lower := strings.ToLower(model)
	for k, p := range pricing {
		if strings.HasPrefix(lower, strings.ToLower(k)) {
			return p
		}
	}
	return defaultPrice
}

// EstimateCost calculates USD cost for a session's token usage.
// When only TotalTokens is available (no input/output breakdown),
// estimates using a 70/30 input/output split as a rough heuristic.
func EstimateCost(model string, t provider.TokenUsage) float64 {
	p := lookupPrice(model)
	m := 1_000_000.0

	if t.InputTokens > 0 || t.OutputTokens > 0 {
		cost := float64(t.InputTokens) / m * p.Input
		cost += float64(t.OutputTokens) / m * p.Output
		cost += float64(t.CacheReadTokens) / m * p.CacheRead
		cost += float64(t.CacheWriteTokens) / m * p.CacheWrite
		return cost
	}

	// Fallback: only TotalTokens available — estimate 70% input, 30% output
	if t.TotalTokens > 0 {
		estInput := float64(t.TotalTokens) * 0.7
		estOutput := float64(t.TotalTokens) * 0.3
		return estInput/m*p.Input + estOutput/m*p.Output
	}

	return 0
}

// Summary holds aggregated cost data.
type Summary struct {
	TotalCostUSD float64                 `json:"totalCostUsd"`
	TotalTokens  int64                   `json:"totalTokens"`
	ByProvider   map[string]ProviderCost `json:"byProvider"`
	ByProject    map[string]ProjectCost  `json:"byProject"`
	Daily        []DailyCost             `json:"daily"`
	Pricing      map[string]ModelPrice   `json:"pricing"`
	Plans        []PlanInfo              `json:"plans"`
}

type ProviderCost struct {
	CostUSD  float64              `json:"costUsd"`
	Tokens   provider.TokenUsage  `json:"tokens"`
	Sessions int                  `json:"sessions"`
}

type ProjectCost struct {
	CostUSD  float64 `json:"costUsd"`
	Sessions int     `json:"sessions"`
	Provider string  `json:"provider"`
}

type DailyCost struct {
	Date    string  `json:"date"`
	CostUSD float64 `json:"costUsd"`
}

// Aggregate computes cost summary from sessions within a date range.
func Aggregate(sessions []provider.Session, since time.Time) Summary {
	s := Summary{
		ByProvider: make(map[string]ProviderCost),
		ByProject:  make(map[string]ProjectCost),
		Pricing:    GetPricingTable(),
		Plans:      Plans,
	}

	dailyMap := make(map[string]float64)

	for _, sess := range sessions {
		if !since.IsZero() && sess.Modified.Before(since) {
			continue
		}

		cost := sess.EstCostUSD
		s.TotalCostUSD += cost
		s.TotalTokens += sess.Tokens.TotalTokens

		// By provider
		pc := s.ByProvider[sess.Provider]
		pc.CostUSD += cost
		pc.Sessions++
		pc.Tokens.InputTokens += sess.Tokens.InputTokens
		pc.Tokens.OutputTokens += sess.Tokens.OutputTokens
		pc.Tokens.CacheReadTokens += sess.Tokens.CacheReadTokens
		pc.Tokens.CacheWriteTokens += sess.Tokens.CacheWriteTokens
		pc.Tokens.TotalTokens += sess.Tokens.TotalTokens
		s.ByProvider[sess.Provider] = pc

		// By project
		proj := s.ByProject[sess.ProjectName]
		proj.CostUSD += cost
		proj.Sessions++
		proj.Provider = sess.Provider
		s.ByProject[sess.ProjectName] = proj

		// Daily
		day := sess.Modified.Format("2006-01-02")
		dailyMap[day] += cost
	}

	for day, cost := range dailyMap {
		s.Daily = append(s.Daily, DailyCost{Date: day, CostUSD: cost})
	}

	return s
}
