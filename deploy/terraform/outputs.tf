output "web_url" {
  description = "Dashboard URL."
  value       = google_cloud_run_v2_service.web.uri
}

output "api_url" {
  description = "GraphQL API base URL (POST /graphql)."
  value       = google_cloud_run_v2_service.api.uri
}

output "worker_url" {
  description = "Private worker URL, reachable by Pub/Sub and Cloud Scheduler only."
  value       = google_cloud_run_v2_service.worker.uri
}

output "events_topic" {
  description = "Topic the storefront publishes events to, with an eventType attribute."
  value       = google_pubsub_topic.events.id
}

output "image_registry" {
  description = "Artifact Registry path for the images."
  value       = local.registry
}
