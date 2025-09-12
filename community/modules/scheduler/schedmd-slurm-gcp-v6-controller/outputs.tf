# Copyright 2024 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

output "slurm_cluster_name" {
  description = "Slurm cluster name."
  value       = local.slurm_cluster_name
}

output "slurm_controller_instance" {
  description = "Compute instance of controller node"
  value       = module.slurm_controller_template[0].self_link
}

output "slurm_login_instances" {
  description = "Compute instances of login nodes"
  value       = flatten([for k, v in module.login : v.instances])
}

output "slurm_bucket_path" {
  description = "Bucket path used by cluster."
  value       = module.slurm_files.slurm_bucket_path
}

output "slurm_bucket_name" {
  description = "GCS Bucket name of Slurm cluster file storage."
  value       = module.slurm_files.bucket_name
}

output "slurm_bucket_dir" {
  description = "Path directory within `bucket_name` for Slurm cluster file storage."
  value       = module.slurm_files.bucket_dir
}


output "instructions" {
  description = "Post deployment instructions."
  value       = <<-EOT
    To SSH to the controller (may need to add '--tunnel-through-iap'):
      
      # Get a controller instance name
      INSTANCE_NAME=$(gcloud compute instance-groups managed list-instances ${google_compute_region_instance_group_manager.controller_mig.name} --region=${var.region} --format="value(instance)" --limit=1)

      # SSH into that instance
      gcloud compute ssh "$INSTANCE_NAME" --zone=$(gcloud compute instances describe "$INSTANCE_NAME" --format="value(zone)")

    If you are using cloud ops agent with this deployment,
    you can use the following command to see the logs for the entire cluster or any particular VM host:
      gcloud logging read labels.cluster_name=${local.slurm_cluster_name}
      gcloud logging read labels.hostname=${local.slurm_cluster_name}-controller
  EOT
}
