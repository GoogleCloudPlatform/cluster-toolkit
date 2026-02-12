## Description

This module creates a [Slinky](https://slinky.ai) cluster and nodesets, for a [Slurm](https://slurm.schedmd.com/documentation.html)-on-Kubernetes HPC setup.

The setup closely follows the [documented quickstart installation v0.3.1](https://github.com/SlinkyProject/slurm-operator/blob/v0.3.1/docs/quickstart.md), with the exception of a more lightweight monitoring/metrics setup. Consider scraping the Slurm Exporter with [Google Managed Prometheus](https://cloud.google.com/stackdriver/docs/managed-prometheus) and a [PodMonitoring resource](https://cloud.google.com/stackdriver/docs/managed-prometheus/setup-managed#gmp-pod-monitoring), rather than a cluster-local Kube Prometheus Stack (although both are possible with module parameterizations). It also provisions a login node (pod).

Through `cert_manager_values`, `prometheus_values`, `slurm_operator_values`, and `slurm_values`, you can customize the Helm releases that constitute Slinky. The Cert Manager, Slurm Operator, and Slurm Helm installations are required, whereas the Prometheus Helm chart is optional (and not included by default). Set `install_kube_prometheus_stack=true` to install Prometheus.

### Example

```yaml
- id: slinky
  source: community/modules/scheduler/slinky
  use: [gke_cluster, base_pool]
  settings:
    slurm_values:
      compute:
        nodesets:
        - name: h3
          enabled: true
          replicas: 2
          image:
            # Use the default nodeset image
            repository: ""
            tag: ""
          resources:
            requests:
              cpu: 86
              memory: 324Gi
            limits:
              cpu: 86
              memory: 324Gi
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                - matchExpressions:
                  - key: "node.kubernetes.io/instance-type"
                    operator: In
                    values:
                    - h3-standard-88
          partition:
            enabled: true
      login: # Login node
        enabled: true
        replicas: 1
        rootSshAuthorizedKeys: []
        image:
          # Use the default login image
          repository: ""
          tag: ""
        resources:
          requests:
            cpu: 500m
            memory: 4Gi
          limits:
            cpu: 500m
            memory: 4Gi
        affinity:
          nodeAffinity:
            requiredDuringSchedulingIgnoredDuringExecution:
              nodeSelectorTerms:
              - matchExpressions:
                - key: "node.kubernetes.io/instance-type"
                  operator: In
                  values:
                  - e2-standard-8 # base_pool's machine-type
```

This creates a Slinky cluster with the following attributes:

* Slinky Helm releases are installed atop the `gke_cluster` (from the `gke-cluster` module).
* Slinky system components and a login node are scheduled on the `base_pool` (from the `gke-node-pool` module).
  * This node affinity specification is recommended, to save HPC hardware for HPC nodesets, and to ensure Helm releases are fully uninstalled before all nodepools are deleted during a `gcluster destroy`.
* One Slurm nodeset is provisioned, with resource requests/limits and node affinities aligned to h3-standard-88 VMs.

### Usage

To test Slurm functionality, connect to the controller or the login node and use Slurm client commands:

```bash
gcloud container clusters get-credentials YOUR_CLUSTER --region YOUR_REGION
```

Connect to the controller:

```bash
kubectl exec -it statefulsets/slurm-controller \
  --namespace=slurm \
  -- bash --login
```

Connect to the login node:

```bash
SLURM_LOGIN_IP="$(kubectl get services -n slurm -l app.kubernetes.io/instance=slurm,app.kubernetes.io/name=login -o jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")"
## Assuming your public SSH key was configured in `login.rootSshAuthorizedKeys[]`.
ssh -p 2222 root@${SLURM_LOGIN_IP}
## Assuming SSSD is configured.
ssh -p 2222 ${USER}@${SLURM_LOGIN_IP}
```

On the connected pod (e.g. host slurm@slurm-controller-0), run the following commands to quickly test if Slurm is functioning:

```bash
sinfo
srun hostname
sbatch --wrap="sleep 60"
squeue
```

<!-- BEGINNING OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | = 1.12.2 |
| <a name="requirement_google"></a> [google](#requirement\_google) | >= 6.16 |
| <a name="requirement_helm"></a> [helm](#requirement\_helm) | ~> 2.17 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | >= 6.16 |
| <a name="provider_helm"></a> [helm](#provider\_helm) | ~> 2.17 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [helm_release.cert_manager](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.prometheus](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.slurm](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [helm_release.slurm_operator](https://registry.terraform.io/providers/hashicorp/helm/latest/docs/resources/release) | resource |
| [google_client_config.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/client_config) | data source |
| [google_container_cluster.gke_cluster](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/container_cluster) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_cert_manager_chart_version"></a> [cert\_manager\_chart\_version](#input\_cert\_manager\_chart\_version) | Version of the Cert Manager chart to install. | `string` | `"v1.18.2"` | no |
| <a name="input_cert_manager_values"></a> [cert\_manager\_values](#input\_cert\_manager\_values) | Value overrides for the Cert Manager release | `any` | <pre>{<br/>  "crds": {<br/>    "enabled": true<br/>  }<br/>}</pre> | no |
| <a name="input_cluster_id"></a> [cluster\_id](#input\_cluster\_id) | An identifier for the GKE cluster resource with format projects/<project\_id>/locations/<region>/clusters/<name>. | `string` | n/a | yes |
| <a name="input_install_kube_prometheus_stack"></a> [install\_kube\_prometheus\_stack](#input\_install\_kube\_prometheus\_stack) | Install the Kube Prometheus Stack. | `bool` | `false` | no |
| <a name="input_install_slurm_chart"></a> [install\_slurm\_chart](#input\_install\_slurm\_chart) | Install slurm-operator chart. | `bool` | `true` | no |
| <a name="input_install_slurm_operator_chart"></a> [install\_slurm\_operator\_chart](#input\_install\_slurm\_operator\_chart) | Install slurm-operator chart. | `bool` | `true` | no |
| <a name="input_node_pool_names"></a> [node\_pool\_names](#input\_node\_pool\_names) | Names of node pools, for use in node affinities (Slinky system components). | `list(string)` | `null` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | The project ID that hosts the GKE cluster. | `string` | n/a | yes |
| <a name="input_prometheus_chart_version"></a> [prometheus\_chart\_version](#input\_prometheus\_chart\_version) | Version of the Kube Prometheus Stack chart to install. | `string` | `"77.0.1"` | no |
| <a name="input_prometheus_values"></a> [prometheus\_values](#input\_prometheus\_values) | Value overrides for the Prometheus release | `any` | <pre>{<br/>  "installCRDs": true<br/>}</pre> | no |
| <a name="input_slurm_chart_version"></a> [slurm\_chart\_version](#input\_slurm\_chart\_version) | Version of the Slurm chart to install. | `string` | `"0.3.1"` | no |
| <a name="input_slurm_namespace"></a> [slurm\_namespace](#input\_slurm\_namespace) | slurm namespace for charts | `string` | `"slurm"` | no |
| <a name="input_slurm_operator_chart_version"></a> [slurm\_operator\_chart\_version](#input\_slurm\_operator\_chart\_version) | Version of the Slurm Operator chart to install. | `string` | `"0.3.1"` | no |
| <a name="input_slurm_operator_namespace"></a> [slurm\_operator\_namespace](#input\_slurm\_operator\_namespace) | slurm namespace for charts | `string` | `"slinky"` | no |
| <a name="input_slurm_operator_repository"></a> [slurm\_operator\_repository](#input\_slurm\_operator\_repository) | Value overrides for the Slinky release | `string` | `"oci://ghcr.io/slinkyproject/charts"` | no |
| <a name="input_slurm_operator_values"></a> [slurm\_operator\_values](#input\_slurm\_operator\_values) | Value overrides for the Slinky release | `any` | `{}` | no |
| <a name="input_slurm_repository"></a> [slurm\_repository](#input\_slurm\_repository) | Value overrides for the Slinky release | `string` | `"oci://ghcr.io/slinkyproject/charts"` | no |
| <a name="input_slurm_values"></a> [slurm\_values](#input\_slurm\_values) | Value overrides for the Slurm release | `any` | `{}` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_slurm_namespace"></a> [slurm\_namespace](#output\_slurm\_namespace) | namespace for the slurm chart |
| <a name="output_slurm_operator_namespace"></a> [slurm\_operator\_namespace](#output\_slurm\_operator\_namespace) | namespace for the slinky operator chart |
<!-- END OF PRE-COMMIT-TERRAFORM DOCS HOOK -->
