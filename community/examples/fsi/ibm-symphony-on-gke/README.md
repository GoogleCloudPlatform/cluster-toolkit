# IBM Spectrum Symphony on Google Kubernetes Engine (GKE)

This project provides a blueprint for deploying an [IBM Spectrum Symphony](https://www.ibm.com/products/spectrum-symphony) cluster on Google Cloud Platform using the [Google Cloud Cluster Toolkit](https://cloud.google.com/cluster-toolkit) and Google Kubernetes Engine (GKE). The blueprint automates infrastructure provisioning, custom base image creation via Packer, Artifact Registry repository creation, GKE cluster setup with the Google Symphony Kubernetes Operator, and Symphony Host Factory integration for elastic containerized compute bursting into GKE.

## Overview

This deployment combines a dedicated IBM Spectrum Symphony Master VM on Google Compute Engine (GCE) with elastic compute workers running as containerized pods on Google Kubernetes Engine (GKE):

*   **Symphony Management Host (GCE Master VM):** Runs IBM Spectrum Symphony Master services (EGO, REST, PMC Web Console) and the Host Factory service configured with the `gcpgke` provider plugin (`hf-gke`).
*   **Google Kubernetes Engine (GKE):** Managed Kubernetes cluster providing scalable compute nodes for Symphony compute workloads, with cluster autoscaling support.
*   **Google Symphony Kubernetes Operator:** Runs inside the GKE cluster, managing custom resources (`GCPSymphonyResource` and `MachineReturnRequest`) to dynamically provision, monitor, and tear down Symphony compute pods.
*   **Artifact Registry:** Managed Docker repository for storing and serving Symphony compute container images (`sym-compute:latest`), provisioned via the official Google Cloud Platform Terraform module.
*   **Cloud Build:** Builds and pushes the containerized Symphony compute image using Cloud BuildKit.
*   **Packer:** Automatically builds the Rocky Linux 8 base image with IBM Spectrum Symphony and the `hf-gke` provider plugin compiled and installed.
*   **Google Cloud Storage (GCS):** Storage bucket holding Symphony binary installers, fixpacks, and entitlement files.

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Symphony Management Host (GCE VM)                     │
│  ┌─────────────────────────┐          ┌──────────────────────────────────┐  │
│  │   IBM Spectrum Symphony │          │       Host Factory Provider      │  │
│  │   Master (EGO / PMC)    │◄────────►│              (hf-gke)            │  │
│  └─────────────────────────┘          └─────────────────┬────────────────┘  │
└─────────────────────────────────────────────────────────┼───────────────────┘
                                                          │ Kubernetes API
                                                          ▼ (Kubeconfig)
┌─────────────────────────────────────────────────────────────────────────────┐
│                      Google Kubernetes Engine (GKE)                         │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                     Google Symphony K8s Operator                      │  │
│  │                                                                       │  │
│  │   • Manages GCPSymphonyResource CRD                                   │  │
│  │   • Manages MachineReturnRequest CRD                                  │  │
│  │   • Reconciles & schedules Symphony Compute Pods                      │  │
│  └──────────────────────────────────┬────────────────────────────────────┘  │
│                                     │                                       │
│                                     ▼                                       │
│     ┌───────────────────────────────────────────────────────────────┐       │
│     │                      Symphony Compute Pods                    │       │
│     │  ┌──────────────────────┐           ┌──────────────────────┐  │       │
│     │  │  sym-compute Pod 1   │           │  sym-compute Pod N   │  │       │
│     │  │  (joins Master VM)   │   • • •   │  (joins Master VM)   │  │       │
│     │  └──────────────────────┘           └──────────────────────┘  │       │
│     └───────────────────────────────────────────────────────────────┘       │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Directory Structure

*   `symphony.yaml`: The main Cluster Toolkit blueprint defining the VPC network, Artifact Registry, GKE cluster, autoscaling GKE node pool, Kubernetes operator deployment via `modules/management/kubectl-apply`, Packer image build, and Master VM.
*   `symphony_deployment.yaml`: Deployment variables configuration file (project ID, region, zone, cluster credentials, GKE node autoscaling limits, and GCS bucket).
*   `resources/`:
    *   `Dockerfile.sym-compute`: Multi-stage Dockerfile for building the Symphony compute worker container image with BuildKit syntax support.
    *   `cloudbuild.yaml`: Cloud Build pipeline for downloading Symphony installers from GCS and building/pushing `sym-compute:latest`.
    *   `manifests/symphony-operator.yaml.tftpl`: Complete Kubernetes manifests template (Namespace, CRDs, RBAC, ServiceAccount, and Deployment) for the Google Symphony Kubernetes Operator.
    *   `pod-specs/pod-spec.yaml`: Kubernetes PodSpec template defining container images, resource requests/limits, and join commands for Symphony worker pods.
    *   `hostProviderPlugins.json`: Registers the `gcpgke` provider plugin with Host Factory.
    *   `hostProviders.json`: Configures the `gcpgkeinst` provider instance.
    *   `hostRequestors.json`: Configures Host Factory requestors (`admin`, `symAinst`) to use `gcpgkeinst`.
    *   `google-ops-agent-config.yaml`: Cloud Ops Agent configuration for monitoring and logging.
*   `scripts/`:
    *   `build_compute_image.sh`: Submits the `sym-compute` container build to Cloud Build and updates `pod-spec.yaml` with the Artifact Registry image URL and master IP.
    *   `install_symphony.sh`: Downloads and installs IBM Spectrum Symphony, fixpack, and entitlements from GCS.
    *   `hf-gke_symphony.sh`: Builds the `hf-gke` provider binary and installs the GKE provider plugin.
    *   `gcpgkeinstprov_config_json.sh`: Configures `gcpgkeinstprov_config.json` with the kubeconfig path and CRD namespace.
    *   `gcpgkeinstprov_templates_json.sh`: Generates `gcpgkeinstprov_templates.json` mapping template IDs to the pod spec.
    *   `setup_kubeconfig.sh`: Generates kubeconfig credentials on the Master VM pointing to the GKE cluster.
    *   `sym_master.sh`: Initializes the Symphony Master node, configures sudoers, and starts EGO services.
    *   `google_ops_agent_config.sh`: Configures Google Cloud Ops Agent logging and metrics.

## Prerequisites

1.  **Google Cloud SDK:** Install and configure the [`gcloud`](https://cloud.google.com/sdk) CLI with appropriate project and region defaults.
2.  **Google Cloud Cluster Toolkit:** Install `gcluster` following the [Cluster Toolkit installation instructions](https://cloud.google.com/cluster-toolkit/docs/setup/install-cluster-toolkit).
3.  **IBM Spectrum Symphony Binaries:** Upload the following Symphony installation files to a Google Cloud Storage bucket:
    *   Symphony installer binary (e.g., `sym-7.3.2.0_x86_64.bin`)
    *   Symphony fixpack (e.g., `sym-7.3.2.0_x86_64_build601711.tar.gz`)
    *   Entitlement file (e.g., `sym_adv_entitlement.dat`)

The GCS bucket layout:

![bucket_image](https://services.google.com/fh/files/misc/data_files.png)

## Deployment

1.  **Configure Deployment Variables:**
    Edit `symphony_deployment.yaml` and set your deployment parameters:

    ```yaml
    vars:
      deployment_name: sym-gke-01
      project_id: <YOUR_PROJECT_ID>
      region: us-central1
      zone: us-central1-c
      authorized_cidr: 0.0.0.0/0
      sym_source_bucket: <YOUR_GCS_BUCKET_NAME>
      sym_installer: sym-7.3.2.0_x86_64.bin
      sym_fixpack: sym-7.3.2.0_x86_64_build601711.tar.gz
      sym_entitlement: sym_adv_entitlement.dat
      symphony_install_dir: /opt/ibm/spectrumcomputing
      admin_password: <ADMIN_PASSWORD>
      gke_node_machine_type: c2-standard-8
      gke_min_node_count: 1
      gke_max_node_count: 100
    ```

2.  **Deploy the Cluster:**
    Run the `gcluster deploy` command:

    ```bash
    gcluster deploy symphony.yaml -d symphony_deployment.yaml --auto-approve
    ```

## Accessing the Cluster

### SSH to Master Node
SSH into the Symphony master VM using IAP:
```bash
gcloud compute ssh "<DEPLOYMENT_NAME>-master-0" --zone "<ZONE>" --project "<PROJECT_ID>" --tunnel-through-iap
```

### Access Symphony Web Management Console (PMC)
Port-forward the PMC web interface (port 8080) to your local machine:
```bash
gcloud compute ssh "<DEPLOYMENT_NAME>-master-0" \
    --zone "<ZONE>" \
    --project "<PROJECT_ID>" \
    --tunnel-through-iap \
    -- -L 8080:localhost:8080
```
Open your browser and navigate to:
```
http://localhost:8080/platform/
```
Log in using:
*   **Username:** `Admin`
*   **Password:** Value configured in `admin_password`

### Verify Host Factory & Cluster Services
On the Master VM:
1. Load Symphony environment and log in:
   ```bash
   source /opt/ibm/spectrumcomputing/profile.platform
   egosh user logon -u Admin -x <ADMIN_PASSWORD>
   ```
2. Verify Symphony cluster and services status:
   ```bash
   egosh resource list
   egosh service list
   ```
3. Test Host Factory GKE provider templates:
   ```bash
   export HF_PROVIDER_CONFDIR=/opt/ibm/spectrumcomputing/hostfactory/conf/providers/gcpgkeinst
   $HF_TOP/1.2/providerplugins/gcpgke/bin/hf-gke getAvailableTemplates
   ```
4. Run a sample ping workload:
   ```bash
   symping -u Admin -x <ADMIN_PASSWORD> -m 10 -r 1000
   ```

## Destroying the Deployment

To cleanly destroy all provisioned cloud resources:
```bash
gcluster destroy <DEPLOYMENT_DIRECTORY> --auto-approve
```
*(Replace `<DEPLOYMENT_DIRECTORY>` with your deployment output directory, e.g. `sym-gke-01`)*