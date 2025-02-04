/**
  * Copyright 2023 Google LLC
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

deployment_name = "golden_copy_deployment"

gpu_zones = ["us-central1-a", "us-central1-b", "us-central1-c", "us-central1-f"]

instance_image_custom = false

labels = {
  ghpc_blueprint  = "versioned"
  ghpc_deployment = "golden_copy_deployment"
}

project_id = "invalid-project"

region = "us-central1"

slurm_image = {
  family  = "slurm-gcp-6-8-hpc-rocky-linux-8"
  project = "schedmd-slurm-public"
}

zone = "us-central1-a"
