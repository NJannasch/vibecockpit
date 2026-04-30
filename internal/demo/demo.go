package demo

import (
	"context"
	"time"

	"vibecockpit/internal/provider"
)

type DemoProvider struct {
	sessions []provider.Session
}

func New() *DemoProvider {
	now := time.Now()
	h := time.Hour
	d := 24 * h

	return &DemoProvider{
		sessions: []provider.Session{
			// Claude Code — active sessions
			{
				ID: "demo-c1", Provider: "claude", ProjectName: "web-dashboard",
				ProjectPath: "~/Projects/web-dashboard", Summary: "Building analytics dashboard with real-time charts",
				Model: "claude-opus-4-6", GitBranch: "main", MessageCount: 847,
				Created: now.Add(-2 * h), Modified: now.Add(-12 * time.Minute), IsActive: true, ActivePID: 10001,
			},
			{
				ID: "demo-c2", Provider: "claude", ProjectName: "payment-service",
				ProjectPath: "~/Projects/payment-service", Summary: "Fix Stripe webhook retry logic — idempotency keys missing",
				Model: "claude-sonnet-4-6", GitBranch: "fix/webhooks", MessageCount: 312,
				Created: now.Add(-4 * h), Modified: now.Add(-2 * h), IsActive: true, ActivePID: 10002,
			},
			{
				ID: "demo-c3", Provider: "claude", ProjectName: "recommendation-engine",
				ProjectPath: "~/Projects/recommendation-engine", Summary: "Implement embedding pipeline for content-based filtering",
				Model: "claude-opus-4-6[1m]", GitBranch: "feat/embeddings", MessageCount: 1523,
				Created: now.Add(-3 * d), Modified: now.Add(-5 * h),
			},
			{
				ID: "demo-c4", Provider: "claude", ProjectName: "auth-service",
				ProjectPath: "~/Projects/auth-service", Summary: "Add OAuth2 PKCE flow for mobile clients",
				Model: "claude-opus-4-6", GitBranch: "feat/pkce", MessageCount: 204,
				Created: now.Add(-120 * d), Modified: now.Add(-1 * d),
			},

			// Codex CLI
			{
				ID: "demo-x1", Provider: "codex", ProjectName: "cloud-infra",
				ProjectPath: "~/Projects/cloud-infra", Summary: "Migrate EKS cluster to new VPC with zero downtime",
				Model: "openai/gpt-5.4", GitBranch: "main", MessageCount: 456,
				Created: now.Add(-1 * h), Modified: now.Add(-45 * time.Minute), IsActive: true, ActivePID: 10003,
			},
			{
				ID: "demo-x2", Provider: "codex", ProjectName: "event-platform",
				ProjectPath: "~/Projects/event-platform", Summary: "Set up Kafka consumers for real-time event processing",
				Model: "openai/gpt-5.4", GitBranch: "feat/kafka", MessageCount: 289,
				Created: now.Add(-75 * d), Modified: now.Add(-8 * h),
			},
			{
				ID: "demo-x3", Provider: "codex", ProjectName: "mobile-app",
				ProjectPath: "~/Projects/mobile-app", Summary: "React Native bottom tabs + deep linking refactor",
				Model: "openai/gpt-5.2", GitBranch: "refactor/nav", MessageCount: 178,
				Created: now.Add(-90 * d), Modified: now.Add(-2 * d),
			},

			// Copilot CLI
			{
				ID: "demo-p1", Provider: "copilot", ProjectName: "file-encryptor",
				ProjectPath: "~/Projects/file-encryptor", Summary: "Build CLI tool with AES-256-GCM encryption",
				Model: "gpt-5-mini", MessageCount: 94,
				Created: now.Add(-150 * d), Modified: now.Add(-3 * h),
			},
			{
				ID: "demo-p2", Provider: "copilot", ProjectName: "shell-config",
				ProjectPath: "~/Projects/shell-config", Summary: "Optimize zsh startup — lazy-load nvm and pyenv",
				Model: "gpt-5-mini", MessageCount: 37,
				Created: now.Add(-200 * d), Modified: now.Add(-1 * d),
			},

			// Gemini CLI
			{
				ID: "demo-g1", Provider: "gemini", ProjectName: "doc-generator",
				ProjectPath: "~/Projects/doc-generator", Summary: "Generate architecture diagrams from codebase analysis",
				Model: "gemini-3-pro", MessageCount: 203,
				Created: now.Add(-40 * d), Modified: now.Add(-4 * h),
			},
			{
				ID: "demo-g2", Provider: "gemini", ProjectName: "email-classifier",
				ProjectPath: "~/Projects/email-classifier", Summary: "Auto-label inbox with custom rules and ML categorization",
				Model: "gemini-3-flash", MessageCount: 156,
				Created: now.Add(-50 * d), Modified: now.Add(-1 * d),
			},

			// OpenCode
			{
				ID: "demo-o1", Provider: "opencode", ProjectName: "home-automation",
				ProjectPath: "~/Projects/home-automation", Summary: "Presence detection + lighting scenes with local LLM",
				Model: "llamacpp/qwen-3.6", MessageCount: 342,
				Created: now.Add(-25 * d), Modified: now.Add(-6 * h),
			},
			{
				ID: "demo-o2", Provider: "opencode", ProjectName: "static-site",
				ProjectPath: "~/Projects/static-site", Summary: "Astro blog with MDX, syntax highlighting, and RSS",
				Model: "anthropic/claude-opus-4-5", MessageCount: 521,
				Created: now.Add(-30 * d), Modified: now.Add(-3 * d),
			},

			// Remote sessions
			{
				ID: "demo-r1", Provider: "remote-gpu-box", ProjectName: "model-training",
				ProjectPath: "/home/deploy/training", Summary: "Fine-tune LoRA adapter on support ticket dataset",
				Model: "claude-opus-4-6", GitBranch: "experiment/lora-v3", MessageCount: 89,
				Created: now.Add(-5 * d), Modified: now.Add(-12 * h),
			},
			{
				ID: "demo-r2", Provider: "remote-gpu-box", ProjectName: "serving-api",
				ProjectPath: "/home/deploy/serving", Summary: "Deploy vLLM with batched inference and health checks",
				Model: "codex/gpt-5.4", MessageCount: 234,
				Created: now.Add(-10 * d), Modified: now.Add(-1 * d),
			},
			{
				ID: "demo-r3", Provider: "remote-agent", ProjectName: "agent-tasks",
				ProjectPath: "~/.agent", Summary: "Automated monitoring — cron health checks via telegram",
				Model: "qwen-3.6-moe", MessageCount: 67,
				Created: now.Add(-15 * d), Modified: now.Add(-20 * time.Minute), IsActive: true, ActivePID: 10004,
			},
		},
	}
}

func (d *DemoProvider) Name() string { return "demo" }
func (d *DemoProvider) Icon() string { return "●" }

func (d *DemoProvider) ScanSessions(_ context.Context) ([]provider.Session, error) {
	return d.sessions, nil
}

func (d *DemoProvider) ResumeCommand(s provider.Session) (string, []string) {
	return "echo", []string{"Demo session:", s.ID}
}

func (d *DemoProvider) NewCommand(dir string) (string, []string) {
	return "echo", []string{"Demo new project:", dir}
}

func (d *DemoProvider) DeleteSession(_ string) error { return nil }
