variable "project_id" {
  description = "Google Cloud project that hosts Pressroom."
  type        = string
}

variable "region" {
  description = "Region for Cloud Run, Cloud SQL and Artifact Registry."
  type        = string
  default     = "us-central1"
}

variable "initial_image" {
  description = <<-EOT
    Image the services start with, before Cloud Build has pushed Pressroom's
    own images. Cloud Build rolls out the real ones and Terraform ignores
    image drift from then on.
  EOT
  type        = string
  default     = "us-docker.pkg.dev/cloudrun/container/hello"
}

variable "db_tier" {
  description = "Cloud SQL machine tier."
  type        = string
  default     = "db-custom-1-3840"
}

variable "db_high_availability" {
  description = "Run Cloud SQL as a regional (HA) instance."
  type        = bool
  default     = false
}

variable "secret_env" {
  description = <<-EOT
    Environment variables loaded from existing Secret Manager secrets, as
    ENV_NAME => secret id. Create each secret and its first version before
    applying, e.g. `printf %s "$KEY" | gcloud secrets create anthropic-api-key --data-file=-`.
  EOT
  type        = map(string)
  default = {
    ANTHROPIC_API_KEY   = "anthropic-api-key"
    OPENAI_API_KEY      = "openai-api-key"
    XAI_API_KEY         = "xai-api-key"
    SLACK_WEBHOOK_URL   = "slack-webhook-url"
    PRESSROOM_API_TOKEN = "pressroom-api-token"
  }
}

variable "public_access" {
  description = "Allow unauthenticated invocations of the API and dashboard. The API still requires its bearer token; set false to front both with IAP instead."
  type        = bool
  default     = true
}

variable "event_publishers" {
  description = "Service accounts (the storefront) allowed to publish storefront events."
  type        = list(string)
  default     = []
}

variable "hourly_rate_usd" {
  description = "Loaded hourly cost of the work agents replace; turns hours saved into dollars."
  type        = number
  default     = 32
}

variable "evaluation_schedule" {
  description = "Cron schedule for the daily crew evaluation."
  type        = string
  default     = "0 6 * * *"
}

variable "time_zone" {
  description = "Time zone for scheduled jobs."
  type        = string
  default     = "America/New_York"
}
