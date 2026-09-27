# Least privilege: one identity per workload, each granted only what it uses.

resource "google_service_account" "api" {
  account_id   = "pressroom-api"
  display_name = "Pressroom API"
}

resource "google_service_account" "worker" {
  account_id   = "pressroom-worker"
  display_name = "Pressroom worker and migrations"
}

resource "google_service_account" "invoker" {
  account_id   = "pressroom-invoker"
  display_name = "Identity Pub/Sub and Cloud Scheduler use to call the worker"
}

locals {
  runtime_accounts = {
    api    = google_service_account.api.email
    worker = google_service_account.worker.email
  }
  secret_grants = {
    for pair in setproduct(keys(local.runtime_accounts), values(var.secret_env)) :
    "${pair[0]}/${pair[1]}" => { account = local.runtime_accounts[pair[0]], secret = pair[1] }
  }
}

resource "google_project_iam_member" "sql_client" {
  for_each = local.runtime_accounts
  project  = var.project_id
  role     = "roles/cloudsql.client"
  member   = "serviceAccount:${each.value}"
}

resource "google_secret_manager_secret_iam_member" "database_url" {
  for_each  = local.runtime_accounts
  secret_id = google_secret_manager_secret.database_url.id
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${each.value}"
}

resource "google_secret_manager_secret_iam_member" "provided" {
  for_each  = local.secret_grants
  secret_id = each.value.secret
  role      = "roles/secretmanager.secretAccessor"
  member    = "serviceAccount:${each.value.account}"
}

# Only the invoker identity may call the worker.
resource "google_cloud_run_v2_service_iam_member" "worker_invoker" {
  name     = google_cloud_run_v2_service.worker.name
  location = var.region
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.invoker.email}"
}

# Pub/Sub mints OIDC tokens for the invoker identity on push.
resource "google_service_account_iam_member" "pubsub_token_creator" {
  service_account_id = google_service_account.invoker.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = local.pubsub_agent
}

# Dead-lettering needs the Pub/Sub service agent on both ends.
resource "google_pubsub_topic_iam_member" "dead_letter_publisher" {
  topic  = google_pubsub_topic.dead_letter.id
  role   = "roles/pubsub.publisher"
  member = local.pubsub_agent
}

resource "google_pubsub_subscription_iam_member" "dead_letter_subscriber" {
  subscription = google_pubsub_subscription.worker_push.id
  role         = "roles/pubsub.subscriber"
  member       = local.pubsub_agent
}

resource "google_pubsub_topic_iam_member" "event_publishers" {
  for_each = toset(var.event_publishers)
  topic    = google_pubsub_topic.events.id
  role     = "roles/pubsub.publisher"
  member   = "serviceAccount:${each.value}"
}

# The API enforces its own bearer token; the dashboard is static files.
resource "google_cloud_run_v2_service_iam_member" "public" {
  for_each = var.public_access ? {
    api = google_cloud_run_v2_service.api.name
    web = google_cloud_run_v2_service.web.name
  } : {}
  name     = each.value
  location = var.region
  role     = "roles/run.invoker"
  member   = "allUsers"
}
