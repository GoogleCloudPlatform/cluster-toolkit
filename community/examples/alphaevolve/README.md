# AlphaEvolve Blueprints

This directory contains declarative [Cluster Toolkit (`gcluster`)](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment) blueprints for deploying and managing the cloud infrastructure required for running scalable AlphaEvolve evolutionary code optimization experiments on Google Cloud.

The core Python framework, user examples, and evaluator modules are located in [`solutions/alphaevolve`](https://github.com/GoogleCloudPlatform/hpc-solutions/tree/main/solutions/alphaevolve) in the [Google Cloud HPC Solutions](https://github.com/GoogleCloudPlatform/hpc-solutions) repository.

---

## What the solution provides

The solution provides an end-to-end flow for the configuration, deployment, and execution of the infrastructure required for AlphaEvolve optimization problems:

* **Configure**: Define the overall configuration of the cloud environment (e.g., region, VM type, consumption model etc). These settings are configured in the [`alpha-evolve-deployment.yaml`](alpha-evolve-deployment.yaml) file.
* **Deploy the Base Infrastructure**: Use Cluster Toolkit to deploy the base infrastructure required for running optimizations. In this step, necessary APIs are activated, service accounts set up, a shared Artifact Registry repository created for the base container image, a Pub/Sub topic, and a Vertex AI Colab Enterprise runtime is provisioned. This base infrastructure can be reused for multiple optimization problems. The infrastructure-as-code for this is defined in [`alpha-evolve-infra.yaml`](alpha-evolve-infra.yaml).
* **Configure the Software Environment**: Configure and deploy the software environment for your specific optimization problem. This Software Environment is defined in [`alpha-evolve-experiment.yaml`](alpha-evolve-experiment.yaml). It uses Cluster Toolkit (which invokes Cloud Build and Artifact Registry) to build a container image that sets up the adequate environment for your code to run in. Experiment containers inherit from the pre-built shared vanilla base image (`alpha-evolve-base:latest`), making experiment container builds fast (2–3 seconds) and cleanly isolating dependencies.

The solution expects a pre-configured GCS bucket where it will store the experiment artifacts. This allows the user to tear down the aforementioned infrastructure when not in use.

## Blueprints Overview

The solution splits infrastructure into reusable base components and per-experiment software environments:

* **[`alpha-evolve-deployment.yaml`](alpha-evolve-deployment.yaml)**: Global configuration settings for your cloud environment (project, region, bucket, and Git repository attributes).
* **[`alpha-evolve-infra.yaml`](alpha-evolve-infra.yaml)**: Deploys the reusable base infrastructure required for running optimizations:
  * Activates required GCP APIs (Cloud Batch, Pub/Sub, Artifact Registry, Discovery Engine, Cloud Build, Vertex AI).
  * Configures dedicated Service Accounts and IAM permissions.
  * Creates an Artifact Registry repository and builds the shared base container image (`alpha-evolve-base:latest`).
  * Creates Pub/Sub notification topics for Batch job tracking.
  * Provisions a Vertex AI Colab Enterprise custom runtime.
* **[`alpha-evolve-experiment.yaml`](alpha-evolve-experiment.yaml)**: Builds and stages the software environment for a specific optimization problem:
  * Invokes Cloud Build to construct problem-specific container images inheriting from `alpha-evolve-base`.
  * Generates and stages Batch job execution templates and environment configurations to Cloud Storage (GCS).
  * Creates Pub/Sub subscriptions for tracking experiment worker events.

---

## Getting Started

### Prerequisites

Ensure you have:

* **Google Cloud SDK** installed:
  * If not installed, follow the [Google Cloud SDK Installation Guide](https://cloud.google.com/sdk/docs/install).
  * For standard Linux distributions (like Debian/Ubuntu), you can use: `sudo apt-get install google-cloud-cli`.
* **Cluster Toolkit (`gcluster`)** installed:
  * For detailed instructions, see the [Cluster Toolkit Documentation](https://docs.cloud.google.com/cluster-toolkit/docs/setup/configure-environment).
  * **Note**: To run `gcluster` from any directory, add the directory containing the `gcluster` binary to your `PATH`. For example:

    ```bash
    export PATH=$PATH:/path/to/cluster-toolkit
    ```

    You can add this line to your `~/.bashrc` (or equivalent) to make it permanent.
* **Your Project ID** added to the AlphaEvolve allow-list.
* **User IAM Permissions**: The Colab notebook runs with user privileges. Users executing the notebook need **GCS Read/Write Access** on the bucket (`roles/storage.objectUser`) as well as **Batch Job Editor** (`roles/batch.jobsEditor`) permissions.

### Billing Info

> **WARNING**: The following resources will be deployed as part of this solution. Billing accrual may occur as resources are created and used.

* Discovery Engine
* Cloud Build
* Cloud Batch
* Artifact Registry
* Cloud Storage
* Compute Engine
* Pub/Sub
* Vertex AI (Colab Enterprise)

To get more information on pricing, see the [pricing page](https://cloud.google.com/products/pricing).

---

## Deploying the Solution with a Preconfigured Example

Follow these steps to deploy the solution for one of the preconfigured examples.

### Authenticate

Make sure that you are logged in to [ADC](https://cloud.google.com/docs/authentication/provide-credentials-adc):

```bash
gcloud auth application-default login
```

### Set up your environment variables & create your GCS Bucket

Set the following environment variables to your desired values:

```bash
export PROJECT_ID=<YOUR_PROJECT_ID>
export REGION=<YOUR_REGION>             # e.g. us-central1
export BUCKET_NAME=<YOUR_BUCKET_NAME>   # must be globally unique
export USER_EMAIL=<YOUR_USER_EMAIL>     # Google Cloud user email (e.g. user@example.com) for Colab Enterprise runtime
```

And then create your GCS bucket (or skip if your bucket already exists):

```bash
gcloud storage buckets create gs://${BUCKET_NAME} --project=${PROJECT_ID} --location=${REGION}
```

**IMPORTANT**: The bucket name has to be globally unique. If your first attempt fails, try another name.

### Deploy the Base Infrastructure

We use Cluster Toolkit to set up the base infrastructure (APIs, GCS bucket, Pub/Sub, Artifact Registry, and the Colab Enterprise runtime).

```bash
gcluster deploy alpha-evolve-infra.yaml -d alpha-evolve-deployment.yaml -o ../deployment \
  --vars project_id=${PROJECT_ID} \
  --vars region=${REGION} \
  --vars existing_bucket_name=${BUCKET_NAME} \
  --vars runtime_user_email=${USER_EMAIL} \
  -w -l IGNORE --auto-approve
```

**IMPORTANT**: The `gcluster deploy` command must be run from the directory containing this README file as shown in the example above.

When completed the output will look something like:

```text
2026-05-01T17:45:04Z Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
2026-05-01T17:45:04Z: Collecting terraform outputs from /home/<user>/deployment/alpha-evolve-infra/colab
2026-05-01T17:45:04Z: Deployment group colab contains no artifacts to export
2026-05-01T17:45:04Z:
###############################
2026-05-01T17:45:04Z: Find instructions for cleanly destroying infrastructure and advanced manual
2026-05-01T17:45:04Z: deployment instructions at:
2026-05-01T17:45:04Z:
2026-05-01T17:45:04Z: /home/<user>/deployment/alpha-evolve-infra/instructions.txt
```

### Create the Software Environment for an experiment

We use Cluster Toolkit to create the software environment for an experiment, i.e., the container image that is used to run (and compile - if appropriate) your code. Under the hood, Cluster Toolkit will use Cloud Build to build the container image and store it in Artifact Registry.

#### Option A: Deploying with a Pre-shipped Example

Choose your example from the [pre-configured problems](https://github.com/GoogleCloudPlatform/hpc-solutions/tree/main/solutions/alphaevolve/user_examples) (e.g. `user_examples/circle_packing_cloud_batch` or `user_examples/adaptive_sort_cpp`):

```bash
export EXAMPLE_DIR=user_examples/circle_packing_cloud_batch    # or any other example directory of the examples above
```

And then run the following command (this will take several minutes to run as it needs to build the container image):

```bash
gcluster deploy alpha-evolve-experiment.yaml -d alpha-evolve-deployment.yaml -o ../deployment \
  --vars project_id=${PROJECT_ID} \
  --vars region=${REGION} \
  --vars existing_bucket_name=${BUCKET_NAME} \
  --vars example_dir=${EXAMPLE_DIR} \
  -w -l IGNORE --auto-approve
```

#### Option B: Deploying with a Custom Local Experiment

When deploying your own custom experiment developed locally on your build machine (not in the remote Git repository):

1. In [`alpha-evolve-experiment.yaml`](alpha-evolve-experiment.yaml), switch `stage-experiment-source` under `group: experiment-setup` from `file-system/gcs-sync` to `file-system/gcs-objects`:

   ```yaml
   - group: experiment-setup
     modules:
     - id: stage-experiment-source
       source: community/modules/file-system/gcs-objects
       settings:
         bucket_name: $(vars.existing_bucket_name)
         directories:
         - source_path: $(vars.example_dir)
           destination_path: $(vars.user_experiment_name)/source
   ```

2. Pass the full directory path to `example_dir` and set `is_example_dir_local=true` on the CLI:

   ```bash
   gcluster deploy alpha-evolve-experiment.yaml -d alpha-evolve-deployment.yaml -o ../deployment \
     --vars project_id=${PROJECT_ID} \
     --vars region=${REGION} \
     --vars existing_bucket_name=${BUCKET_NAME} \
     --vars example_dir="/absolute/path/to/my_custom_exp/" \
     --vars is_example_dir_local=true \
     --vars user_experiment_name="my-custom-exp" \
     -w -l IGNORE --auto-approve
   ```

   Cloud Build automatically packages and uploads your local directory as a build overlay to build container images in the cloud, while `gcs-objects` uploads your local source files directly to Cloud Storage.

> **IMPORTANT**
> The `gcluster deploy` command must be run from the directory containing this README file as shown in the example above.

When complete, the output will look something like:

```text

====================================================================
==== How to run your experiment in the Notebook ====
All configurations have been natively saved to GCS under: gs://<bucket_name>/<user_experiment_name>/

Please open the Jupyter Notebook and follow these steps under the 'Show how change experiment run' section:

1. Go to the section: 'Adjust environment variables on the Notebook'.
2. Run the interactive cell: 'Run this cell to specify the experiment...'.
3. Select the option corresponding to: '<user_experiment_name>'.
   This will dynamically load all of your GCS environment variables!
====================================================================
2026-05-01T18:13:29Z module.show-instructions.null_resource.gcloud_commands (local-exec): + printf %b $';33m====================================================================\n==== How to run your experiment in the Notebook ====\nAll configurations have been natively saved to GCS under: gs://<bucket_name>/<user_experiment_name>/\n\nPlease open the Jupyter Notebook and follow these steps under the \'Show how change experiment run\' section:\n\n1. Go to the section: \'Adjust environment variables on the Notebook\'.\n2. Run the interactive cell: \'Run this cell to specify the experiment...\'.\n3. Select the option corresponding to: \'<user_experiment_name>\'.\n   This will dynamically load all of your GCS environment variables!\n====================================================================\033[0m\n'
2026-05-01T18:13:29Z module.show-instructions.null_resource.gcloud_commands (local-exec): + echo 'All gcloud CREATE commands executed successfully for show-instructions.'
2026-05-01T18:13:29Z module.show-instructions.null_resource.gcloud_commands (local-exec): All gcloud CREATE commands executed successfully for show-instructions.
2026-05-01T18:13:29Z module.show-instructions.null_resource.gcloud_commands: Creation complete after 0s [id=1881624798606908193]

2026-05-01T18:13:29Z Apply complete! Resources: 1 added, 0 changed, 0 destroyed.
2026-05-01T18:13:29Z: Collecting terraform outputs from /home/<user>/deployment/alpha-evolve-experiment-experiment-1/show-instructions
2026-05-01T18:13:29Z: Deployment group show-instructions contains no artifacts to export
```

## Run the experiment

Now you can connect to Vertex AI Colab Enterprise and run the experiment.

1. **Open Vertex AI Colab Enterprise**:
   * Navigate to the Google Cloud Console and go to [**Vertex AI** -> **Colab Enterprise**](https://console.cloud.google.com/vertex-ai/colab/notebooks) (or direct link [here](https://console.cloud.google.com/vertex-ai/colab/notebooks)).
   * Make sure you have selected the correct **Project** and **Region** matching your deployment configuration.
2. **Import the notebook from Cloud Storage**:
   * Click the **Import** button (represented by an upload icon) at the top of the page.
   * In the **Import notebooks** dialog, select **Cloud Storage** as the import source.
   * Browse your GCS bucket (configured as `BUCKET_NAME` in your deployment) and select the notebook file at `notebook/run_notebook.ipynb` (i.e., `gs://<YOUR_BUCKET_NAME>/notebook/run_notebook.ipynb`).
   * Click **Import**. The notebook will now appear under your **My notebooks** list. Click on it to open it.
3. **Connect to your custom runtime**:
   * Open the notebook.
   * Click the **Connect** dropdown in the top-right corner of the Colab Enterprise interface and select **Connect to custom runtime**.
   * Select the custom runtime provisioned by your deployment (named after your `deployment_name`, e.g., `alpha-evolve-infra` or the custom name you configured).
   * Click **Connect**.
4. **Run the controller**:
   * Run the cells in the notebook to start the controller container and begin your experiment. The notebook is configured to dynamically discover available experiments from GCP project metadata and load your environment variables from GCS via an interactive prompt.
5. **Validate Successful Run**:
   * You can monitor candidate scores and progress directly in the notebook cell execution output, specifically by executing cells under the 'Process Results Plot Optimization' section.
6. **Find Generated Code and Results**:
   * All candidate code files and corresponding JSON evaluation results are stored in the GCS bucket under the 'archive' folder, organized by individual program candidate. You can also view these from the Colab notebook by checking the `data/` directory.

---

## Where to go from here?

**If you want to end your experimentation...**
... tear down your infrastructure. See section [Cleanup and Data Access](#cleanup-and-data-access) for instructions on how to do this. To stop a running experiment, simply delete the `current_experiment.json` file located under your `user_experiment_name` directory within the GCS Bucket. There is no need to submit any API call to AlphaEvolve to terminate the experiment.

**If you want to pause your experimentation for a while...**
... note that while Cloud Batch evaluation worker VMs only spin up on demand during active experiment runs, the **Colab Enterprise runtime (`n2-standard-4`) remains active** and incurs compute charges while running. An automatic **180-minute (3-hour) idle shutdown** is enabled on the runtime to power down inactive VMs and prevent continuous billing if left unattended. To halt billing immediately when inactive, simply disconnect or stop the runtime session from Colab. To resume experimentation later, re-connect to the runtime session in Jupyter / Colab. The experiment state is saved in `current_experiment.json` on GCS, allowing paused experiments to resume seamlessly from where they left off. Note that if an experiment encounters an unrecoverable failure and enters a `FAILED` state, the platform automatically deletes `current_experiment.json` so that re-running the controller initiates a fresh, clean restart.

## Cleanup and Data Access

Once you are done with your experimentations, you can destroy the resources.

To review all the provisioned workspaces staged for deployment:

```bash
ls ../deployment/
```

For each workspace (e.g., `alpha-evolve-infra`, `ae-exp-experiment-1`), run the destroy command from this directory:

```bash
gcluster destroy ../deployment/<workspace_name> --auto-approve
```

> [!CAUTION]
> Your generated program candidate codes and evaluation results will remain accessible in your GCS bucket even after other infrastructure is destroyed. **This means the GCS bucket will remain active and continue to incur charges.** To avoid this, you should manually delete the bucket to stop incurring fees. If you wish to retain the files, you MUST download the content to your private system before deleting the bucket.
