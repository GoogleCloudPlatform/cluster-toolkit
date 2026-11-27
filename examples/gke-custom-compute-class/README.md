# GKE Custom Compute Classes (CCC) Blueprint Example

This blueprint demonstrates how to deploy a Google Kubernetes Engine (GKE)
cluster with Node Auto-Provisioning (NAP) and configure **GKE Custom Compute
Classes (`ComputeClass` CRDs)** using the
`modules/management/custom-compute-class` module.

It illustrates two complementary patterns for defining compute classes:

1. **Inline-defined Compute Class**: Fully declared within the blueprint settings using structured HCL/YAML parameters (demonstrating priority lists with Spot instance fallback, taints, and labels).
2. **Template-defined Compute Class**: Parameterized through an external Terraform template (`.tftpl`) file via `custom_manifest_path` and `custom_template_vars`.

## Architecture Overview

```text
                        +------------------------------------------+
                        |               GKE Cluster                |
                        |      (Node Auto-Provisioning Enabled)    |
                        +--------------------+---------------------+
                                             |
             +-------------------------------+-------------------------------+
             |                                                               |
             v                                                               v
+----------------------------+                                 +----------------------------+
| ComputeClass:              |                                 | ComputeClass:              |
| spot-priority-compute-class|                                 | template-defined-compute-..|
+----------------------------+                                 +----------------------------+
| * Source: inline settings  |                                 | * Source: custom .tftpl    |
| * Priority 1: C3 Spot      |                                 | * Machine family: C3       |
| * Priority 2: N2 On-demand |                                 | * Min cores / memory       |
| * Taints & Labels          |                                 | * Zonal location pinning   |
+----------------------------+                                 +----------------------------+
             |                                                               |
             v                                                               v
[ Pod with nodeSelector:     ]                                 [ Pod with nodeSelector:     ]
[ cloud.google.com/          ]                                 [ cloud.google.com/          ]
[   compute-class:           ]                                 [   compute-class:           ]
[   spot-priority-..         ]                                 [   template-defined-..      ]
```

## Blueprint Components

* **`network`** (`modules/network/vpc`): Creates a VPC and subnet with secondary ranges for GKE pods and services.
* **`gke_cluster`** (`modules/scheduler/gke-cluster`): Creates a GKE cluster with `cluster_autoscaling` enabled for Node Auto-Provisioning.
* **`ccc_inline`** (`modules/management/custom-compute-class`):
  Applies `spot-priority-compute-class` defining prioritized machine
  families (`c3` Spot with `n2` on-demand fallback) and custom node
  labels/taints.
* **`ccc_template`** (`modules/management/custom-compute-class`):
  Applies `template-defined-compute-class` rendered from
  `./templates/custom-compute-class.yaml.tftpl`.

## Prerequisites

* [Cluster Toolkit (`gcluster`) CLI](https://cloud.google.com/cluster-toolkit/docs/setup/install-toolkit) installed.
* A GCP project with billing enabled.
* Permissions to create GKE clusters, VPC networks, and IAM bindings.

## Deployment Instructions

1. **Deploy the Blueprint**:

   ```bash
   gcluster deploy ./gke-custom-compute-class.yaml \
     --vars project_id=<YOUR_PROJECT_ID>
   ```

2. **Connect to the Cluster**:

   ```bash
   gcloud container clusters get-credentials ccc-demo \
     --region us-central1 \
     --project <YOUR_PROJECT_ID> \
     --dns-endpoint
   ```

3. **Verify Compute Classes**:

   ```bash
   kubectl get computeclasses
   ```

   Output should display both `spot-priority-compute-class` and
   `template-defined-compute-class`.

4. **Deploy Sample Workloads**:

   Apply the sample pods provided in `sample-workloads.yaml`:

   ```bash
   kubectl apply -f ./sample-workloads.yaml
   ```

   Check that GKE Node Auto-Provisioning creates nodes matching each compute
   class:

   ```bash
   kubectl get pods -o wide
   kubectl get nodes -L cloud.google.com/compute-class,workload-tier
   ```

## Cleanup

Destroy all resources created by this blueprint:

```bash
gcluster destroy ccc-demo
```
