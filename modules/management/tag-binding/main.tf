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

resource "google_tags_location_tag_binding" "binding" {
  # Create a unique key by joining parent, tag_value, and location.
  # This prevents key collisions when one parent has multiple tags.
  for_each = {
    for item in var.tag_binding :
    md5("${item.parent}-${item.tag_value}-${item.location}") => item
  }

  parent    = each.value.parent
  tag_value = each.value.tag_value
  location  = each.value.location
}
