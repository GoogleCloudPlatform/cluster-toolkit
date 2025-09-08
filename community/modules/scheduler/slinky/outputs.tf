# Copyright 2025 "Google LLC"
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

output "instructions" {
  description = "Post deployment instructions."
  value       = <<-EOT
    To test Slurm functionality, connect to the controller to use Slurm client commands:
      kubectl exec -it statefulsets/slurm-controller \
        --namespace=slurm \
        -- bash --login

    On the controller pod (e.g. host slurm@slurm-controller-0), run the following commands to quickly test Slurm is functioning:
      sinfo
      srun hostname
      sbatch --wrap="sleep 60"
      squeue
  EOT
}

output "slurm_namespace" {
  description = "namespace for the slurm chart"
  value       = var.slurm_namespace
}

output "slurm_operator_namespace" {
  description = "namespace for the slinky operator chart"
  value       = var.slurm_operator_namespace
}
