/*
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

# Search for all keys under the parent
data "google_tags_tag_keys" "existing_tag_keys" {
  parent = var.tag_key_parent
}

locals {
  # We use try() to handle cases where the provider hasn't
  # populated the list yet during validation.
  all_keys = try(data.google_tags_tag_keys.existing_tag_keys.keys, [])

  # Filter by short_name
  matching_keys = [
    for k in local.all_keys : k
    if k.short_name == var.tag_key_short_name
  ]

  # Determine existence and capture the ID
  tag_key_exists = length(local.matching_keys) > 0

  # If it exists, take the ID from the data source;
  # If not, take it from the resource we are about to create.
  # google_tags_tag_key.key will have a length of 1 only if tag_key_exists is false.
  tag_key_id = local.tag_key_exists ? local.matching_keys[0].name : (length(google_tags_tag_key.key) > 0 ? google_tags_tag_key.key[0].id : null)
}

# Create the key only if it wasn't found
resource "google_tags_tag_key" "key" {
  count = local.tag_key_exists ? 0 : 1

  parent       = var.tag_key_parent
  short_name   = var.tag_key_short_name
  description  = var.tag_key_description
  purpose      = var.tag_key_purpose
  purpose_data = var.tag_key_purpose_data
}

# Create values using the resolved ID
resource "google_tags_tag_value" "values" {
  for_each = { for val in var.tag_values : val.short_name => val }

  parent     = local.tag_key_id
  short_name = each.value.short_name
  # Use coalesce to provide a default "" if each.value.description is null
  description = coalesce(each.value.description, "")
}
