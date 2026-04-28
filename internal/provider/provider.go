package provider

import (
	"context"
	"time"
)

type TokenUsage struct {
	InputTokens     int64 `json:"inputTokens"`
	OutputTokens    int64 `json:"outputTokens"`
	CacheReadTokens int64 `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64 `json:"cacheWriteTokens,omitempty"`
	ReasoningTokens int64 `json:"reasoningTokens,omitempty"`
	TotalTokens     int64 `json:"totalTokens"`
}

type Session struct {
	ID           string     `json:"id"`
	Provider     string     `json:"provider"`
	ProjectName  string     `json:"projectName"`
	ProjectPath  string     `json:"projectPath"`
	Summary      string     `json:"summary,omitempty"`
	FirstPrompt  string     `json:"firstPrompt,omitempty"`
	Model        string     `json:"model,omitempty"`
	GitBranch    string     `json:"gitBranch,omitempty"`
	MessageCount int        `json:"messageCount"`
	Tokens       TokenUsage `json:"tokens,omitempty"`
	EstCostUSD   float64    `json:"estCostUsd,omitempty"`
	Modified     time.Time  `json:"modified"`
	Created      time.Time  `json:"created"`
	IsActive     bool       `json:"isActive"`
	ActivePID    int        `json:"activePID,omitempty"`
	DataPath     string     `json:"-"`
}

type Provider interface {
	Name() string
	Icon() string
	ScanSessions(ctx context.Context) ([]Session, error)
	ResumeCommand(session Session) (bin string, args []string)
	NewCommand(dir string) (bin string, args []string)
	DeleteSession(sessionID string) error
}
