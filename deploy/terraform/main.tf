# Pressroom on Google Cloud:
#
#   dashboard (Cloud Run) ──► api (Cloud Run) ──► Cloud SQL for PostgreSQL
#                                                   ▲
#   storefront ──► Pub/Sub ──push (OIDC)──► worker (Cloud Run, private) ──► Claude / OpenAI / Grok
#   Cloud Scheduler ──(OIDC)──────────────► worker /jobs/evaluate
#
# Secrets live in Secret Manager and reach the containers as environment
# variables. Model API keys are created outside Terraform so their values
# never land in state.

terraform {
  required_version = ">= 1.6"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }

  # terraform init -backend-config="bucket=<state-bucket>"
  backend "gcs" {
    prefix = "pressroom"
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

data "google_project" "this" {}

locals {
  apis = [
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "cloudscheduler.googleapis.com",
    "iam.googleapis.com",
    "pubsub.googleapis.com",
    "run.googleapis.com",
    "secretmanager.googleapis.com",
    "sqladmin.googleapis.com",
  ]

  registry     = "${var.region}-docker.pkg.dev/${var.project_id}/${google_artifact_registry_repository.pressroom.repository_id}"
  pubsub_agent = "serviceAccount:service-${data.google_project.this.number}@gcp-sa-pubsub.iam.gserviceaccount.com"

  # Settings shared by the API and the worker.
  common_env = {
    PRESSROOM_ENV              = "production"
    PRESSROOM_MODEL_MODE       = "live"
    GOOGLE_CLOUD_PROJECT       = var.project_id
    LOG_LEVEL                  = "info"
    PRESSROOM_HOURLY_RATE_USD  = tostring(var.hourly_rate_usd)
    PRESSROOM_EVAL_WINDOW_DAYS = "30"
  }
}

resource "google_project_service" "apis" {
  for_each           = toset(local.apis)
  service            = each.value
  disable_on_destroy = false
}

resource "google_artifact_registry_repository" "pressroom" {
  repository_id = "pressroom"
  location      = var.region
  format        = "DOCKER"
  description   = "Pressroom container images"

  cleanup_policies {
    id     = "keep-recent"
    action = "KEEP"
    most_recent_versions {
      keep_count = 20
    }
  }

  depends_on = [google_project_service.apis]
}
