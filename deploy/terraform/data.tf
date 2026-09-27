# ---- Cloud SQL for PostgreSQL ----

resource "google_sql_database_instance" "main" {
  name                = "pressroom"
  database_version    = "POSTGRES_17"
  region              = var.region
  deletion_protection = true

  settings {
    tier              = var.db_tier
    edition           = "ENTERPRISE"
    availability_type = var.db_high_availability ? "REGIONAL" : "ZONAL"

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
    }

    # Cloud Run reaches the instance through the Cloud SQL connector (a Unix
    # socket), so no authorized networks are opened.
    ip_configuration {
      ipv4_enabled = true
      ssl_mode     = "ENCRYPTED_ONLY"
    }

    insights_config {
      query_insights_enabled = true
    }
  }

  depends_on = [google_project_service.apis]
}

resource "google_sql_database" "pressroom" {
  name     = "pressroom"
  instance = google_sql_database_instance.main.name
}

resource "random_password" "db" {
  length  = 32
  special = false
}

resource "google_sql_user" "app" {
  name     = "pressroom"
  instance = google_sql_database_instance.main.name
  password = random_password.db.result
}

# ---- Secret Manager ----

resource "google_secret_manager_secret" "database_url" {
  secret_id = "pressroom-database-url"

  replication {
    auto {}
  }

  depends_on = [google_project_service.apis]
}

resource "google_secret_manager_secret_version" "database_url" {
  secret      = google_secret_manager_secret.database_url.id
  secret_data = "postgres://pressroom:${random_password.db.result}@/pressroom?host=/cloudsql/${google_sql_database_instance.main.connection_name}"
}

# ---- Pub/Sub: storefront events with a dead-letter topic ----

resource "google_pubsub_topic" "events" {
  name                       = "storefront-events"
  message_retention_duration = "86400s"
  depends_on                 = [google_project_service.apis]
}

resource "google_pubsub_topic" "dead_letter" {
  name       = "storefront-events-dead-letter"
  depends_on = [google_project_service.apis]
}

resource "google_pubsub_subscription" "worker_push" {
  name                 = "pressroom-worker-push"
  topic                = google_pubsub_topic.events.id
  ack_deadline_seconds = 60

  push_config {
    push_endpoint = "${google_cloud_run_v2_service.worker.uri}/events/pubsub"
    oidc_token {
      service_account_email = google_service_account.invoker.email
      audience              = google_cloud_run_v2_service.worker.uri
    }
  }

  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.dead_letter.id
    max_delivery_attempts = 10
  }

  expiration_policy {
    ttl = ""
  }
}

resource "google_pubsub_subscription" "dead_letter_inspection" {
  name                       = "storefront-events-dead-letter-inspection"
  topic                      = google_pubsub_topic.dead_letter.id
  message_retention_duration = "604800s"
}

# ---- Cloud Scheduler: daily crew evaluation ----

resource "google_cloud_scheduler_job" "evaluate" {
  name             = "pressroom-evaluate-crew"
  description      = "Apply the retirement policy to every agent on duty."
  schedule         = var.evaluation_schedule
  time_zone        = var.time_zone
  attempt_deadline = "320s"

  retry_config {
    retry_count = 3
  }

  http_target {
    http_method = "POST"
    uri         = "${google_cloud_run_v2_service.worker.uri}/jobs/evaluate"
    oidc_token {
      service_account_email = google_service_account.invoker.email
      audience              = google_cloud_run_v2_service.worker.uri
    }
  }

  depends_on = [google_project_service.apis]
}
