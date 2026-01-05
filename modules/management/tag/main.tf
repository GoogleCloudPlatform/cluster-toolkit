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

# Use the external data source
data "external" "check_tag_key" {
  program = ["bash", "${path.module}/check_tag.sh"]

  query = {
    parent     = var.tag_key_parent
    short_name = var.tag_key_short_name
  }
}

locals {
  # If the script returned an empty string, the key doesn't exist
  tag_key_exists = data.external.check_tag_key.result.id != ""
  tag_key_id     = local.tag_key_exists ? data.external.check_tag_key.result.id : google_tags_tag_key.key[0].id
}

resource "google_tags_tag_key" "key" {
  count = local.tag_key_exists ? 0 : 1

  parent       = var.tag_key_parent
  short_name   = var.tag_key_short_name
  description  = var.tag_key_description
  purpose      = var.tag_key_purpose
  purpose_data = var.tag_key_purpose_data
}

resource "google_tags_tag_value" "values" {
  # This creates a map where the key is the short_name 
  # and the value is the entire object
  for_each = { for val in var.tag_value : val.short_name => val }

  parent      = local.tag_key_id
  short_name  = each.value.short_name
  description = each.value.description
}
