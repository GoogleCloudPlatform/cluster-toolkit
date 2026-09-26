/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

resource "google_discovery_engine_data_store" "default" {
  project                     = var.project_id
  location                    = var.location
  data_store_id               = "${var.engine_id}-ds"
  display_name                = "${var.engine_id}-ds"
  industry_vertical           = "GENERIC"
  content_config              = "NO_CONTENT"
  solution_types              = ["SOLUTION_TYPE_SEARCH"]
  create_advanced_site_search = false
}

resource "google_discovery_engine_search_engine" "engine" {
  project           = var.project_id
  location          = var.location
  collection_id     = var.collection
  engine_id         = var.engine_id
  display_name      = var.engine_id
  industry_vertical = "GENERIC"
  data_store_ids    = [google_discovery_engine_data_store.default.data_store_id]
  search_engine_config {
    search_tier    = "SEARCH_TIER_ENTERPRISE"
    search_add_ons = ["SEARCH_ADD_ON_LLM"]
  }
}

resource "google_discovery_engine_assistant" "assistant" {
  project            = var.project_id
  location           = var.location
  collection_id      = var.collection
  engine_id          = google_discovery_engine_search_engine.engine.engine_id
  assistant_id       = var.assistant_id
  display_name       = var.assistant_id
  web_grounding_type = "WEB_GROUNDING_TYPE_UNSPECIFIED"
}
