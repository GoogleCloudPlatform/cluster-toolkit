# IBM Spectrum Symphony on Google Kubernetes Engine (GKE)

This project provides a blueprint for deploying an [IBM Spectrum Symphony](https://www.ibm.com/products/spectrum-symphony) cluster on Google Cloud Platform using the [Google Cloud Cluster Toolkit](https://cloud.google.com/cluster-toolkit) and Google Kubernetes Engine (GKE). The blueprint automates infrastructure provisioning, custom base image creation via Packer, Artifact Registry repository creation, GKE cluster setup with the Google Symphony Kubernetes Operator, and Symphony Host Factory integration for elastic containerized compute bursting into GKE.

## Overview

This deployment combines a dedicated IBM Spectrum Symphony Master VM on Google Compute Engine (GCE) with elastic compute workers running as containerized pods on Google Kubernetes Engine (GKE):

* **Symphony Management Host (GCE Master VM):** Runs IBM Spectrum Symphony Master services (EGO, REST, PMC Web Console) and the Host Factory service configured with the `gcpgke` provider plugin (`hf-gke`).
* **Google Kubernetes Engine (GKE):** Managed Kubernetes cluster providing scalable compute nodes for Symphony compute workloads, with cluster autoscaling support.
* **Google Symphony Kubernetes Operator:** Runs inside the GKE cluster, managing custom resources (`GCPSymphonyResource` and `MachineReturnRequest`) to dynamically provision, monitor, and tear down Symphony compute pods.
* **Artifact Registry:** Managed Docker repository for storing and serving Symphony compute container images (`sym-compute:latest`), provisioned via the Cluster Toolkit Artifact Registry module (`community/modules/container/artifact-registry`).
* **Google Cloud Secret Manager:** Securely stores the Symphony `Admin` password (`<DEPLOYMENT_NAME>-admin-password`) and provides it to the Master VM at runtime via `gcloud secrets`, preventing plaintext password exposure in VM metadata or process logs.
* **Cloud Build:** Builds and pushes the containerized Symphony compute image using Cloud BuildKit.
* **Packer:** Automatically builds the Rocky Linux 8 base image with IBM Spectrum Symphony and the `hf-gke` provider plugin compiled and installed.
* **Google Cloud Storage (GCS):** Storage bucket holding Symphony binary installers, fixpacks, and entitlement files.

## Architecture

```text
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

* `symphony.yaml`: The main Cluster Toolkit blueprint defining Secret Manager secret verification (`verify_admin_password_secret`), API enablement, the VPC network, service accounts, Artifact Registry, GKE cluster, autoscaling GKE node pool, Kubernetes operator deployment via `modules/management/kubectl-apply`, Packer image build, and Master VM.
* `symphony_deployment.yaml`: Deployment variables configuration file (project ID, region, zone, Secret Manager secret name, GKE node autoscaling limits, and GCS bucket).
* `resources/`:
  * `Dockerfile.sym-compute`: Multi-stage Dockerfile for building the Symphony compute worker container image with BuildKit syntax support.
  * `cloudbuild.yaml`: Cloud Build pipeline for downloading Symphony installers from GCS and building/pushing `sym-compute:latest`.
  * `manifests/symphony-operator.yaml.tftpl`: Complete Kubernetes manifests template (Namespace, CRDs, RBAC, ServiceAccount, and Deployment) for the Google Symphony Kubernetes Operator.
  * `pod-specs/pod-spec.yaml`: Kubernetes PodSpec template defining container images, resource requests/limits (`4` CPU, `16384Mi` memory), and join environment variables for Symphony worker pods.
  * `hostProviderPlugins.json`: Registers the `gcpgke` provider plugin with Host Factory.
  * `hostProviders.json`: Configures the `gcpgkeinst` provider instance.
  * `hostRequestors.json`: Configures Host Factory requestors (`admin`, `symAinst`) to use `gcpgkeinst`.
* `scripts/`:
  * `build_compute_image.sh`: Submits the `sym-compute` container build to Cloud Build and pushes the image to Artifact Registry.
  * `install_symphony.sh`: Downloads and installs IBM Spectrum Symphony, fixpack, and entitlements from GCS.
  * `hf-gke_symphony.sh`: Clones the pinned `symphony-gcp` release (`v1.0.4`), builds the `hf-gke` provider binary, and installs the GKE provider plugin.
  * `hostfactoryconf_json.sh`: Generates `hostfactoryconf.json` with Host Factory logging, polling, and REST settings.
  * `gcpgkeinstprov_config_json.sh`: Configures `gcpgkeinstprov_config.json` with the kubeconfig path and CRD namespace.
  * `gcpgkeinstprov_templates_json.sh`: Updates `pod-spec.yaml` with the Artifact Registry image URL and Master VM IP/hostname, and generates `gcpgkeinstprov_templates.json` mapping template IDs to the pod spec.
  * `setup_kubeconfig.sh`: Generates kubeconfig credentials on the Master VM pointing to the GKE cluster.
  * `sym_master.sh`: Initializes the Symphony Master node, configures sudoers, retrieves the Symphony Admin password from Secret Manager using `gcloud secrets`, and starts EGO services.
  * `google_ops_agent_config.sh`: Configures Google Cloud Ops Agent logging and metrics.

## Prerequisites

* **Google Cloud SDK:** Install and configure the [`gcloud`](https://cloud.google.com/sdk) CLI with appropriate project and region defaults.
* **Google Cloud Cluster Toolkit:** Install `gcluster` following the [Cluster Toolkit installation instructions](https://cloud.google.com/cluster-toolkit/docs/setup/install-cluster-toolkit).
* **IBM Spectrum Symphony Binaries:** Upload the following Symphony installation files to a Google Cloud Storage bucket:
  * Symphony installer binary (e.g., `sym-7.3.2.0_x86_64.bin`)
  * Symphony fixpack (e.g., `sym-7.3.2.0_x86_64_build601711.tar.gz`)
  * Entitlement file (e.g., `sym_adv_entitlement.dat`)

The GCS bucket layout:

![bucket_image](https://services.google.com/fh/files/misc/data_files.png)

* **Google Cloud Secret Manager & IAM Setup:**
  To avoid storing or passing the Symphony `Admin` password in plaintext, the password must be stored in a Google Cloud Secret Manager secret (by default `<DEPLOYMENT_NAME>-admin-password`, configured via `admin_password_secret`). The blueprint verifies that this secret exists during the `primary` deployment stage (`verify_admin_password_secret`) and exits immediately if it has not been created. During Master VM startup, `scripts/sym_master.sh` retrieves the password at runtime using `gcloud secrets`.

  1. Ensure the Secret Manager API is enabled and your deployer identity has permissions to create and read secrets (`roles/secretmanager.admin` and `roles/secretmanager.secretAccessor`, as basic roles like `roles/writer` or `roles/editor` do not include `secretmanager.versions.access`):

     ```bash
     gcloud services enable secretmanager.googleapis.com --project="<YOUR_PROJECT_ID>"

     gcloud projects add-iam-policy-binding "<YOUR_PROJECT_ID>" \
         --member="user:<DEPLOYER_USER_EMAIL>" \
         --role="roles/secretmanager.admin"

     gcloud projects add-iam-policy-binding "<YOUR_PROJECT_ID>" \
         --member="user:<DEPLOYER_USER_EMAIL>" \
         --role="roles/secretmanager.secretAccessor"
     ```

     *(Note: If deploying with a service account, replace `user:<DEPLOYER_USER_EMAIL>` with `serviceAccount:<DEPLOYER_SERVICE_ACCOUNT_EMAIL>`.)*

  2. Create the admin password secret before deploying:

     ```bash
     printf "<ADMIN_PASSWORD>" | gcloud secrets create "<DEPLOYMENT_NAME>-admin-password" \
         --data-file=- \
         --replication-policy="automatic" \
         --project="<YOUR_PROJECT_ID>"
     ```

     *(To rotate or update an existing secret with a new version later, use `gcloud secrets versions add`)*:

     ```bash
     printf "<NEW_ADMIN_PASSWORD>" | gcloud secrets versions add "<DEPLOYMENT_NAME>-admin-password" \
         --data-file=- \
         --project="<YOUR_PROJECT_ID>"
     ```

## Deployment

1. **Configure Deployment Variables:**
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
     admin_password_secret: sym-gke-01-admin-password
     gke_node_machine_type: c2-standard-4
     gke_min_node_count: 1
     gke_max_node_count: 100
   ```

   > [!IMPORTANT]
   > Make sure the secret specified in `admin_password_secret` (e.g., `sym-gke-01-admin-password`) has been created in Secret Manager prior to running `gcluster deploy`. The `verify_admin_password_secret` step in the `primary` group checks that the secret exists and has an accessible version, and will abort the deployment if it is missing.

2. **Deploy the Cluster:**
   Run the `gcluster deploy` command:

   ```bash
   gcluster deploy symphony.yaml -d symphony_deployment.yaml --auto-approve
   ```

   > [!TIP]
   > If the custom Packer image has already been built and you only want to update or re-provision the cluster infrastructure, you can skip the image build step by adding `--skip packer`:
   >
   > ```bash
   > gcluster deploy symphony.yaml -d symphony_deployment.yaml -w --auto-approve --skip packer
   > ```

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
    -- -L 8888:localhost:8080
```

Open your browser and navigate to:

```text
http://localhost:8888/platform/
```

Log in using:

* **Username:** `Admin`
* **Password:** Password stored in Secret Manager (retrieve via `gcloud secrets versions access latest --secret="<DEPLOYMENT_NAME>-admin-password" --project="<PROJECT_ID>"`)

### Verify Host Factory & Cluster Services

On the Master VM:

1. Load Symphony environment and log in using the password stored in Secret Manager:

   ```bash
   source /opt/ibm/spectrumcomputing/profile.platform
   ADMIN_PASSWORD=$(gcloud secrets versions access latest --secret="<DEPLOYMENT_NAME>-admin-password")
   egosh user logon -u Admin -x "$ADMIN_PASSWORD"
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
   symping -u Admin -x "$ADMIN_PASSWORD" -m 10 -r 1000
   ```

### Requesting & Returning Hosts

While `egosh` manages EGO services and cluster resources once worker pods join, Host Factory provisioning is triggered via the Host Factory REST API, the `hf-gke` CLI, or workload demand (`symping`):

1. **Verify the Host Factory Service and REST Endpoint with `egosh`:**

   ```bash
   egosh service list -l | grep HostFactory
   egosh client view REST_HOST_FACTORY_URL
   ```

2. **Request Compute Pods via the Host Factory REST API (`admin` requestor):**

   ```bash
   curl -X POST -u "Admin:${ADMIN_PASSWORD}" \
     -H "Content-Type: application/json" \
     -d '{
       "demand_hosts": [
         {
           "prov_name": "gcpgkeinst",
           "template_name": "sym-pod-c2-4",
           "ninstances": 2
         }
       ]
     }' \
     http://localhost:8080/platform/rest/hostfactory/requestor/admin/request
   ```

3. **Alternative: Request Compute Pods Directly via the `hf-gke` Provider CLI:**
   Pass the JSON request payload file using `-f`:

   ```bash
   export HF_PROVIDER_CONFDIR=/opt/ibm/spectrumcomputing/hostfactory/conf/providers/gcpgkeinst

   cat << 'EOF' > /tmp/request.json
   {
     "template": {
       "templateId": "sym-pod-c2-4",
       "machineCount": 1
     }
   }
   EOF

   $HF_TOP/1.2/providerplugins/gcpgke/bin/hf-gke requestMachines -f /tmp/request.json
   ```

4. **Monitor and Return Hosts with `egosh`:**

   ```bash
   # List worker pods that have joined the Symphony cluster
   egosh resource list -l

   # Close and remove a worker host in EGO, then return it via Host Factory
   egosh resource close -reclaim <HOST_NAME>
   egosh resource remove <HOST_NAME>

   curl -X DELETE -u "Admin:${ADMIN_PASSWORD}" \
     -H "Content-Type: application/json" \
     -d '["<HOST_NAME>"]' \
     http://localhost:9080/platform/rest/hostfactory/requestor/admin/hosts

   unset ADMIN_PASSWORD
   ```

## Destroying the Deployment

To cleanly destroy all provisioned cloud resources:

```bash
gcluster destroy <DEPLOYMENT_DIRECTORY> --auto-approve
```

*(Replace `<DEPLOYMENT_DIRECTORY>` with your deployment output directory, e.g. `sym-gke-01`)*
