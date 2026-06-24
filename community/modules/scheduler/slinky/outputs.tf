# Copyright 2026 "Google LLC"
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

output "slurm_namespace" {
  description = "namespace for the slurm chart"
  value       = var.slurm_namespace
  depends_on = [
    helm_release.cert_manager,
    helm_release.slurm_operator,
    helm_release.slurm,
    helm_release.prometheus
  ]
}

output "slurm_operator_namespace" {
  description = "namespace for the slinky operator chart"
  value       = var.slurm_operator_namespace
  depends_on = [
    helm_release.cert_manager,
    helm_release.slurm_operator,
    helm_release.slurm,
    helm_release.prometheus
  ]
}

output "instructions" {
  description = "Instructions on how to connect to the cluster and run basic Slurm commands."
  value       = <<-EOT
    To test Slurm functionality, connect to the controller or the login node and use Slurm client commands.

    First, get cluster credentials:
      gcloud container clusters get-credentials ${local.cluster_name} --location ${local.cluster_location} --project ${local.project_id}

    Connect to the controller:
      kubectl exec -it statefulsets/slurm-controller --namespace=${var.slurm_namespace} -- bash --login

    Connect to the login node (if enabled):
      (Note: It may take a few minutes for the LoadBalancer IP to become available)
      SLURM_LOGIN_IP="$(kubectl get services -n ${var.slurm_namespace} -l app.kubernetes.io/instance=slurm,app.kubernetes.io/name=login -o jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")"
      # If using root with SSH authorized keys:
      ssh -p 2222 root@$${SLURM_LOGIN_IP}
      # Or if SSSD/LDAP is configured:
      ssh -p 2222 $${USER}@$${SLURM_LOGIN_IP}

    Once connected, you can run:
      sinfo
      srun hostname
  EOT
  depends_on = [
    helm_release.cert_manager,
    helm_release.slurm_operator,
    helm_release.slurm,
    helm_release.prometheus
  ]
}
