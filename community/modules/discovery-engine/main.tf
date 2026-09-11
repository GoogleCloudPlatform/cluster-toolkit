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

resource "terraform_data" "engine" {
  input = {
    project_id = var.project_id
    location   = var.location
    collection = var.collection
    base_url   = var.base_url
    engine_id  = var.engine_id
  }

  triggers_replace = [
    var.project_id,
    var.location,
    var.collection,
    var.base_url,
    var.engine_id,
  ]

  provisioner "local-exec" {
    interpreter = ["/bin/bash"]
    command     = "${path.module}/scripts/manage_discovery_engine.sh"

    environment = {
      ACTION     = "create"
      TARGET     = "engine"
      PROJECT_ID = var.project_id
      LOCATION   = var.location
      COLLECTION = var.collection
      BASE_URL   = var.base_url
      ENGINE_ID  = var.engine_id
    }
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["/bin/bash"]
    command     = "${path.module}/scripts/manage_discovery_engine.sh"

    environment = {
      ACTION     = "destroy"
      TARGET     = "engine"
      PROJECT_ID = self.input.project_id
      LOCATION   = self.input.location
      COLLECTION = self.input.collection
      BASE_URL   = self.input.base_url
      ENGINE_ID  = self.input.engine_id
    }
  }
}

resource "terraform_data" "assistant" {
  depends_on = [terraform_data.engine]

  input = {
    project_id   = var.project_id
    location     = var.location
    collection   = var.collection
    base_url     = var.base_url
    engine_id    = var.engine_id
    assistant_id = var.assistant_id
  }

  triggers_replace = [
    terraform_data.engine.id,
    var.project_id,
    var.location,
    var.collection,
    var.base_url,
    var.engine_id,
    var.assistant_id,
  ]

  provisioner "local-exec" {
    interpreter = ["/bin/bash"]
    command     = "${path.module}/scripts/manage_discovery_engine.sh"

    environment = {
      ACTION       = "create"
      TARGET       = "assistant"
      PROJECT_ID   = var.project_id
      LOCATION     = var.location
      COLLECTION   = var.collection
      BASE_URL     = var.base_url
      ENGINE_ID    = var.engine_id
      ASSISTANT_ID = var.assistant_id
    }
  }

  provisioner "local-exec" {
    when        = destroy
    interpreter = ["/bin/bash"]
    command     = "${path.module}/scripts/manage_discovery_engine.sh"

    environment = {
      ACTION       = "destroy"
      TARGET       = "assistant"
      PROJECT_ID   = self.input.project_id
      LOCATION     = self.input.location
      COLLECTION   = self.input.collection
      BASE_URL     = self.input.base_url
      ENGINE_ID    = self.input.engine_id
      ASSISTANT_ID = self.input.assistant_id
    }
  }
}
