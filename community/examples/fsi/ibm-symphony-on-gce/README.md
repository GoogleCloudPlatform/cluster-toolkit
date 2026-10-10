# IBM Spectrum Symphony on Google Compute Engine (GCE)

This project provides a blueprint for deploying an [IBM Spectrum Symphony](https://www.ibm.com/products/spectrum-symphony) cluster on Google Cloud Platform using the [Google Cloud Cluster Toolkit](https://cloud.google.com/cluster-toolkit). The blueprint automates infrastructure provisioning, custom base image creation via Packer, and Symphony cluster installation with dynamic cloud bursting to Compute Engine Managed Instance Groups (MIGs) via Symphony Host Factory.

## Overview

This blueprint deploys an IBM Spectrum Symphony cluster consisting of a dedicated master VM, a static compute VM, and dynamic compute nodes managed via Compute Engine Managed Instance Groups (MIGs). Dynamic scaling is coordinated through Symphony's Host Factory framework integrated with Google Cloud services:

* **Google Compute Engine (GCE):** Virtual machines for master and compute nodes, with MIGs for autoscaled worker nodes.
* **Google Cloud Storage (GCS):** Storage bucket holding Symphony binary installers, fixpacks, and entitlement files.
* **Google Cloud Secret Manager:** Securely stores the Symphony `Admin` password (`<DEPLOYMENT_NAME>-admin-password`) and provides it to the Master VM at runtime via `gcloud secrets`, preventing plaintext password exposure in VM metadata or process logs.
* **Packer:** Automated creation of a custom Rocky Linux 8 base image pre-installed with Symphony and the GCP Host Factory provider plugin (`hf-gce`).
* **Cloud Logging & Cloud Pub/Sub:** Cloud audit log sink routing GCE instance lifecycle events to Pub/Sub for real-time Host Factory monitoring (`hf-monitor`).

## Architecture

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Symphony Management Host (GCE Master VM)              │
│  ┌─────────────────────────┐          ┌──────────────────────────────────┐  │
│  │   IBM Spectrum Symphony │          │       Host Factory Provider      │  │
│  │   Master (EGO / PMC)    │◄────────►│              (hf-gce)            │  │
│  └─────────────────────────┘          └─────────────────┬────────────────┘  │
└─────────────────────────────────────────────────────────┼───────────────────┘
                                                          │ GCE Compute API &
                                                          │ Pub/Sub Events
                                                          ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Google Compute Engine (GCE)                           │
│  ┌───────────────────────────────┐     ┌─────────────────────────────────┐  │
│  │      Static Compute Nodes     │     │      Managed Instance Groups    │  │
│  │  ┌─────────────────────────┐  │     │   (Dynamic Autoscaled Nodes)    │  │
│  │  │   sym-compute-vm        │  │     │  ┌────────────┐ ┌────────────┐  │  │
│  │  │   (joins Master VM)     │  │     │  │ sym-mig-4  │ │ sym-mig-30 │  │  │
│  │  └─────────────────────────┘  │     │  └────────────┘ └────────────┘  │  │
│  └───────────────────────────────┘     └─────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Directory Structure

* `symphony.yaml`: The main Cluster Toolkit blueprint defining Secret Manager secret verification (`verify_admin_password_secret`), the VPC network, Pub/Sub topic/subscription, Audit Log Sink, Packer image build, MIGs, static compute VM, and Master VM deployment.
* `symphony_deployment.yaml`: Deployment configuration file specifying deployment-specific variables (project ID, region, zone, GCS bucket, Secret Manager secret name, and service account).
* `resources/`: Configuration files and templates (Cloud Ops Agent configuration, Host Factory provider/requestor JSON files, and MIG template generator script `sym_mig_provider.py`).
* `scripts/`: Shell scripts executed during Packer build and VM startup (Symphony installation, Host Factory setup, cluster initialization with Secret Manager password retrieval via `gcloud secrets`, and node join scripts).

## Prerequisites

* **Google Cloud SDK:** Install and configure the [`gcloud`](https://cloud.google.com/sdk) CLI with appropriate credentials.
* **Google Cloud Cluster Toolkit:** Install `gcluster` following the [Cluster Toolkit installation instructions](https://cloud.google.com/cluster-toolkit/docs/setup/install-cluster-toolkit).
* **IBM Spectrum Symphony Binaries:** Upload the following Symphony installation files to a Google Cloud Storage bucket:
  * Symphony installer binary (e.g., `sym-7.3.2.0_x86_64.bin`)
  * Symphony fixpack (e.g., `sym-7.3.2.0_x86_64_build601711.tar.gz`)
  * Entitlement file (e.g., `sym_adv_entitlement.dat`)

The GCS bucket layout should look like:

![bucket_image](https://services.google.com/fh/files/misc/data_files.png)

* **Google Cloud Secret Manager & Deployer IAM Roles:**
  To avoid storing or passing the Symphony `Admin` password in plaintext, the password must be stored in a Google Cloud Secret Manager secret (by default `<DEPLOYMENT_NAME>-admin-password`, configured via `admin_password_secret`). The blueprint verifies that this secret exists during the `primary` deployment stage (`verify_admin_password_secret`) and exits immediately if it has not been created. During Master VM startup, `scripts/sym_master.sh` retrieves the password at runtime using `gcloud secrets`.

  1. The identity executing `gcluster deploy` requires permissions to configure Cloud Audit Log exports, manage Pub/Sub topics, create/read Secret Manager secrets, and assign project IAM roles. In addition to basic resource provisioning permissions (such as `roles/editor` or `roles/writer`), grant the following roles to the deployer identity and ensure the Secret Manager API is enabled:
     * `roles/logging.admin` (or `roles/logging.configWriter`) — to create the Cloud Audit Log sink.
     * `roles/pubsub.admin` — to manage Pub/Sub topic IAM policies for log export ingestion.
     * `roles/secretmanager.admin` and `roles/secretmanager.secretAccessor` — to create, manage, and read the `<DEPLOYMENT_NAME>-admin-password` secret in Secret Manager (basic roles like `roles/writer` or `roles/editor` do not include `secretmanager.versions.access`).
     * `roles/resourcemanager.projectIamAdmin` — to assign the Pub/Sub subscriber and Secret Manager secret accessor roles to the Compute service account.

     Run the following commands using the `gcloud` CLI:

     ```bash
     gcloud services enable secretmanager.googleapis.com --project="<YOUR_PROJECT_ID>"

     gcloud projects add-iam-policy-binding "<YOUR_PROJECT_ID>" \
         --member="user:<DEPLOYER_USER_EMAIL>" \
         --role="roles/logging.admin"

     gcloud projects add-iam-policy-binding "<YOUR_PROJECT_ID>" \
         --member="user:<DEPLOYER_USER_EMAIL>" \
         --role="roles/pubsub.admin"

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
   Edit `symphony_deployment.yaml` and set your deployment variables:

   ```yaml
   vars:
     deployment_name: symphony-01
     project_id: <YOUR_PROJECT_ID>
     region: us-central1
     zone: us-central1-c
     sym_source_bucket: <YOUR_GCS_BUCKET_NAME>
     sym_installer: sym-7.3.2.0_x86_64.bin
     sym_fixpack: sym-7.3.2.0_x86_64_build601711.tar.gz
     sym_entitlement: sym_adv_entitlement.dat
     symphony_install_dir: /opt/ibm/spectrumcomputing
     admin_password_secret: symphony-01-admin-password
     service_account_email: <SERVICE_ACCOUNT_EMAIL>
     pubsub_subscription: hf-gce-vm-events-sub
     pubsub_topic: hf-gce-vm-events
     sink_name: hf-gce-vm-events-sink
   ```

   > [!IMPORTANT]
   > Make sure the secret specified in `admin_password_secret` (e.g., `symphony-01-admin-password`) has been created in Secret Manager prior to running `gcluster deploy`. The `verify_admin_password_secret` step in the `primary` group checks that the secret exists and has an accessible version, and will abort the deployment if it is missing.

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

3. Test Host Factory GCE provider templates:

   ```bash
   export HF_PROVIDER_CONFDIR=/opt/ibm/spectrumcomputing/hostfactory/conf/providers/gcpgceinst
   $HF_TOP/1.2/providerplugins/gcpgce/bin/hf-gce getAvailableTemplates
   ```

4. Run a sample ping workload:

   ```bash
   symping -u Admin -x "$ADMIN_PASSWORD" -m 10 -r 1000
   ```

### Requesting & Returning Hosts

While `egosh` manages EGO services and cluster resources once worker VMs join, Host Factory provisioning is triggered via the Host Factory REST API, the `hf-gce` CLI, or workload demand (`symping`):

1. **Verify the Host Factory Service and REST Endpoint with `egosh`:**

   ```bash
   egosh service list | grep HostFactory
   egosh client view REST_HOST_FACTORY_URL
   ```

2. **Request Compute Hosts via the Host Factory REST API (`admin` requestor):**

   ```bash
   curl -X POST -u "Admin:${ADMIN_PASSWORD}" \
     -H "Content-Type: application/json" \
     -d '{
       "demand_hosts": [
         {
           "prov_name": "gcpgceinst",
           "template_name": "sym-mig-c2-4",
           "ninstances": 2
         }
       ]
     }' \
     http://localhost:9080/platform/rest/hostfactory/requestor/admin/request
   ```

3. **Alternative: Request Compute Hosts Directly via the `hf-gce` Provider CLI:**
   Pass the JSON request payload file using `-f`:

   ```bash
   export HF_PROVIDER_CONFDIR=/opt/ibm/spectrumcomputing/hostfactory/conf/providers/gcpgceinst

   cat << 'EOF' > /tmp/request.json
   {
     "template": {
       "templateId": "sym-mig-c2-4",
       "machineCount": 1
     }
   }
   EOF

   $HF_TOP/1.2/providerplugins/gcpgce/bin/hf-gce requestMachines -f /tmp/request.json
   ```

4. **Monitor and Return Hosts with `egosh`:**

   ```bash
   # List worker VMs that have joined the Symphony cluster
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

*(Replace `<DEPLOYMENT_DIRECTORY>` with your deployment output directory, e.g. `./symphony-01`)*
