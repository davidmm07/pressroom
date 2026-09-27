// Package config reads settings from the environment (twelve-factor). On
// Cloud Run, secrets arrive as environment variables mounted from Secret
// Manager, so the application never talks to Secret Manager itself.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is every setting the binaries use.
type Config struct {
	Env        string // development or production
	Port       string
	LogLevel   string
	LogFormat  string // json or text
	GCPProject string

	DatabaseURL string

	// Models
	ModelMode         string // live, sandbox or auto
	AnthropicAPIKey   string
	OpenAIAPIKey      string
	OpenAIBaseURL     string
	XAIAPIKey         string
	XAIBaseURL        string
	OpenSourceBaseURL string // vLLM or Ollama, OpenAI-compatible
	OpenSourceAPIKey  string
	PricebookJSON     string

	// Measurement
	HourlyRateUSD  float64
	EvalWindowDays int

	// Worker
	WorkerConcurrency int
	PollInterval      time.Duration
	Lease             time.Duration
	MaxAttempts       int

	// API
	APIToken        string
	TrustIAP        bool
	CORSOrigins     []string
	Introspection   bool
	ComplexityLimit int

	// Integrations
	ImageStudioURL   string
	ImageStudioToken string
	SlackWebhookURL  string
}

// Load reads the environment and reports every problem at once.
func Load() (Config, error) {
	var errs []error
	e := env{errs: &errs}
	c := Config{
		Env:        e.str("PRESSROOM_ENV", "development"),
		Port:       e.str("PORT", "8080"),
		LogLevel:   e.str("LOG_LEVEL", "info"),
		LogFormat:  e.str("LOG_FORMAT", "json"),
		GCPProject: e.str("GOOGLE_CLOUD_PROJECT", ""),

		DatabaseURL: e.str("DATABASE_URL", ""),

		ModelMode:         e.str("PRESSROOM_MODEL_MODE", "auto"),
		AnthropicAPIKey:   e.str("ANTHROPIC_API_KEY", ""),
		OpenAIAPIKey:      e.str("OPENAI_API_KEY", ""),
		OpenAIBaseURL:     e.str("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		XAIAPIKey:         e.str("XAI_API_KEY", ""),
		XAIBaseURL:        e.str("XAI_BASE_URL", "https://api.x.ai/v1"),
		OpenSourceBaseURL: e.str("OPEN_SOURCE_BASE_URL", ""),
		OpenSourceAPIKey:  e.str("OPEN_SOURCE_API_KEY", ""),
		PricebookJSON:     e.str("PRESSROOM_PRICEBOOK_JSON", ""),

		HourlyRateUSD:  e.float("PRESSROOM_HOURLY_RATE_USD", 32),
		EvalWindowDays: e.int("PRESSROOM_EVAL_WINDOW_DAYS", 30),

		WorkerConcurrency: e.int("WORKER_CONCURRENCY", 4),
		PollInterval:      e.duration("WORKER_POLL_INTERVAL", time.Second),
		Lease:             e.duration("WORKER_LEASE", 5*time.Minute),
		MaxAttempts:       e.int("WORKER_MAX_ATTEMPTS", 6),

		APIToken:        e.str("PRESSROOM_API_TOKEN", ""),
		TrustIAP:        e.bool("PRESSROOM_TRUST_IAP", false),
		CORSOrigins:     e.list("PRESSROOM_CORS_ORIGINS", "http://localhost:5173"),
		Introspection:   e.bool("GRAPHQL_INTROSPECTION", true),
		ComplexityLimit: e.int("GRAPHQL_COMPLEXITY_LIMIT", 600),

		ImageStudioURL:   e.str("IMAGE_STUDIO_URL", ""),
		ImageStudioToken: e.str("IMAGE_STUDIO_TOKEN", ""),
		SlackWebhookURL:  e.str("SLACK_WEBHOOK_URL", ""),
	}

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if c.WorkerConcurrency < 1 || c.WorkerConcurrency > 64 {
		errs = append(errs, errors.New("WORKER_CONCURRENCY must be between 1 and 64"))
	}
	if c.EvalWindowDays < 1 || c.EvalWindowDays > 365 {
		errs = append(errs, errors.New("PRESSROOM_EVAL_WINDOW_DAYS must be between 1 and 365"))
	}
	if c.Production() && c.APIToken == "" && !c.TrustIAP {
		errs = append(errs, errors.New("production needs PRESSROOM_API_TOKEN or PRESSROOM_TRUST_IAP=true"))
	}
	return c, errors.Join(errs...)
}

// Production reports whether this is a production deployment.
func (c Config) Production() bool { return c.Env == "production" }

type env struct{ errs *[]error }

func (e env) str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return def
}

func (e env) int(key string, def int) int {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*e.errs = append(*e.errs, fmt.Errorf("%s: %q is not an integer", key, v))
	}
	return n
}

func (e env) float(key string, def float64) float64 {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		*e.errs = append(*e.errs, fmt.Errorf("%s: %q is not a number", key, v))
	}
	return f
}

func (e env) bool(key string, def bool) bool {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*e.errs = append(*e.errs, fmt.Errorf("%s: %q is not a boolean", key, v))
	}
	return b
}

func (e env) duration(key string, def time.Duration) time.Duration {
	v := e.str(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*e.errs = append(*e.errs, fmt.Errorf("%s: %q is not a duration such as 30s", key, v))
	}
	return d
}

func (e env) list(key, def string) []string {
	var out []string
	for _, part := range strings.Split(e.str(key, def), ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
