# IBM Spectrum Symphony on Google Cloud

This project provides a blueprint for deploying an [IBM Spectrum Symphony](https://www.ibm.com/products/spectrum-symphony) cluster on Google Cloud Platform using the [Google Cloud Cluster Toolkit](https://cloud.google.com/cluster-toolkit). The blueprint automates infrastructure provisioning, custom image creation via Packer, and Symphony cluster installation with dynamic cloud bursting via Host Factory.

## Overview

This blueprint deploys an IBM Spectrum Symphony cluster consisting of a dedicated master node, static compute nodes, and dynamic compute nodes managed via Compute Engine Managed Instance Groups (MIGs). Dynamic scaling is coordinated through Symphony's Host Factory framework integrated with Google Cloud services:

*   **Google Compute Engine (GCE):** Virtual machines for master and compute nodes, with MIGs for autoscaled worker nodes.
*   **Google Cloud Storage (GCS):** Storage bucket holding Symphony binary installers, fixpacks, and entitlement files.
*   **Packer:** Automated creation of a custom Rocky Linux 8 base image pre-installed with Symphony and the GCP Host Factory provider plugin.
*   **Cloud Logging & Cloud Pub/Sub:** Cloud audit log sink routing GCE instance lifecycle events to Pub/Sub for real-time Host Factory monitoring.

## Directory Structure

*   `symphony.yaml`: The main Cluster Toolkit blueprint defining the infrastructure, Packer image build, MIGs, and VM deployment.
*   `symphony_deployment.yaml`: Deployment configuration file specifying deployment-specific variables (project, region, bucket, credentials, etc.).
*   `resources/`: Configuration files and templates (Cloud Ops Agent configuration, Host Factory provider/requestor JSON files, and MIG template generator script).
*   `scripts/`: Shell scripts executed during Packer build and VM startup (Symphony installation, Host Factory setup, cluster initialization, and node join scripts).

## Prerequisites

1.  **Google Cloud SDK:** Install and configure the [`gcloud`](https://cloud.google.com/sdk) CLI with appropriate credentials.
2.  **Google Cloud Cluster Toolkit:** Install `gcluster` following the [Cluster Toolkit installation instructions](https://cloud.google.com/cluster-toolkit/docs/setup/install-cluster-toolkit).
3.  **IBM Spectrum Symphony Binaries:** Upload the following Symphony installation files to a Google Cloud Storage bucket:
    *   Symphony installer binary (e.g., `sym-7.3.2.0_x86_64.bin`)
    *   Symphony fixpack (e.g., `sym-7.3.2.0_x86_64_build601711.tar.gz`)
    *   Entitlement file (e.g., `sym_adv_entitlement.dat`)

The GCS bucket layout should look like:

![bucket_image](https://services.google.com/fh/files/misc/data_files.png)

## Deployment

1.  **Configure Deployment Variables:**
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
      admin_password: <ADMIN_PASSWORD>
      service_account_email: <SERVICE_ACCOUNT_EMAIL>
    ```

2.  **Deploy the Cluster:**
    Run the `gcluster deploy` command:

    ```bash
    gcluster deploy symphony.yaml -d symphony_deployment.yaml --auto-approve
    ```

    > [!TIP]
    > If the custom Packer image has already been built and you only want to update or re-provision the cluster infrastructure, you can skip the image build step by adding `--skip packer`:
    > ```bash
    > gcluster deploy symphony.yaml -d symphony_deployment.yaml -w --auto-approve --skip packer
    > ```

## Accessing the Cluster

### SSH to Master Node
SSH into the Symphony master VM:
```bash
gcloud compute ssh "<DEPLOYMENT_NAME>-master-0" --zone "<ZONE>" --project "<PROJECT_ID>"
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

## Destroying the Deployment

To cleanly destroy all provisioned cloud resources:
```bash
gcluster destroy <DEPLOYMENT_DIRECTORY> --auto-approve
```
*(Replace `<DEPLOYMENT_DIRECTORY>` with your deployment output directory, e.g. `./symphony-01`)*
