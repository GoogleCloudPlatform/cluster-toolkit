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

image_family = "$(zebra/to(ad"

image_name = "((cat /dog))"

labels = {
  brown           = "$(fox)"
  ghpc_blueprint  = "text_escape"
  ghpc_deployment = "golden_copy_deployment"
  ñred            = "ñblue"
}

project_id = "invalid-project"

subnetwork_name = "$(purple"

zone = "us-east4-c"
