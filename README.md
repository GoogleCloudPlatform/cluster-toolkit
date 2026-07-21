# Google Cluster Toolkit (formerly HPC Toolkit)

## Description

[Cluster Toolkit](https://cloud.google.com/cluster-toolkit) is an open-source software provided by Google Cloud that makes it easy to deploy AI/ML and high performance computing (HPC) environments following Google Cloud best practices.

Cluster Toolkit is highly customizable and extensible, addressing the deployment needs of a broad range of workloads, such as compute, networking and storage, in a repeatable manner.

## Detailed documentation and main components

Cluster Toolkit comes with a suite of [tutorials](docs/tutorials/README.md), [examples](examples/README.md), and full documentation for [modules](modules/README.md) designed for AI/ML and HPC use cases.

The main components of the Cluster Toolkit include:

- **Cluster Blueprint**: A YAML file that defines the cluster's infrastructure and configuration.
- **Modules**: Reusable building blocks (Terraform or Packer) used to compose a blueprint.
- **gcluster engine**: The command-line tool that processes blueprints to create a deployment folder.
- **Deployment Folder**: A self-contained folder containing the Terraform or Packer code needed to provision the environment.

More information can be found on the [Google Cloud documentation site](https://cloud.google.com/cluster-toolkit/docs/overview).

## AI Hypercomputer

The Cluster Toolkit is an integral part of [Google Cloud AI Hypercomputer](https://cloud.google.com/ai-hypercomputer/docs). Documentation for AI Hypercomputer solutions is available for [GKE](https://cloud.google.com/ai-hypercomputer/docs/create/gke-ai-hypercompute) and for [Slurm](https://cloud.google.com/ai-hypercomputer/docs/create/create-slurm-cluster).

## Quickstart

To get started with Cluster Toolkit quickly, we recommend that you follow the quickstart tutorial [Deploy an HPC cluster with Slurm](https://cloud.google.com/cluster-toolkit/docs/quickstarts/slurm-cluster).

## Install Cluster Toolkit

To create a cluster using the Cluster Toolkit, you can use either Cloud Shell, or a workstation that is running Linux or macOS.

Cloud Shell is an interactive development and operations environment that is accessible from your web browser. If you use Cloud Shell, the following dependencies are already pre-installed and you don't need to manually install dependencies.

If you want to work from a Linux or macOS client or workstation, you must follow the steps in [Install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) before you continue.

You can use two different methods to install Cluster Toolkit:

- [Using the Pre-built Bundle (Recommended)](#using-the-pre-built-bundle-recommended)
- [Building from Source](#building-from-source)

### Using the Pre-built Bundle (Recommended)

For the easiest setup, download the appropriate bundle for your operating system and architecture (e.g., `gcluster_bundle_linux_amd64.zip`, `gcluster_bundle_linux_arm64.zip`, `gcluster_bundle_mac_amd64.zip`, or `gcluster_bundle_mac_arm64.zip`) from the [Releases](https://github.com/GoogleCloudPlatform/cluster-toolkit/releases) page. These bundles include the pre-compiled `gcluster` binary, the `examples` folder, and the `community/examples` folder.

#### Bundle Compatibility Matrix

The pre-built bundles are compiled for Linux and macOS execution environments and support the deployment of the following cluster operating systems.

##### Execution Platform (Where the binary runs)

| Platform | Support Status | Notes |
| :--- | :---: | :--- |
| **Linux (amd64 / arm64)** | ✅ | Pre-compiled on Debian Bullseye. Includes amd64 (x86_64) and arm64 builds starting v1.85.0. |
| **Google Cloud Shell** | ✅ | Native support via the Linux amd64 binary. |
| **macOS (amd64 / arm64)** | ✅ | Native support via the Mac binary. Includes amd64 (Intel) and arm64 (Apple Silicon) builds starting v1.85.0. |
| **Windows** | ❎ | Please [Build from Source](#building-from-source). |

1. Download and extract the bundle:

    > **_NOTE:_** Multi-architecture builds (amd64 and arm64) are available starting with version 1.85.0. Tarball bundles (.tgz) are supported starting with version 1.89.0.

    For versions v1.89.0 and newer (Multi-architecture Tarball):

    ```shell
    # Find all available releases at: https://github.com/GoogleCloudPlatform/cluster-toolkit/releases
    # Set the desired version TAG (e.g., v1.89.0)
    TAG=vX.Y.Z
    # Set your OS (linux or mac) and Architecture (amd64 or arm64)
    OS="linux"
    ARCH="amd64"
    # Download and extract the platform-specific bundle in a single step
    mkdir -p cluster-toolkit && curl -L https://github.com/GoogleCloudPlatform/cluster-toolkit/releases/download/${TAG}/gcluster_bundle_${OS}_${ARCH}.tgz | tar -xz -C cluster-toolkit && cd cluster-toolkit
    ```

    For versions v1.85.0 through v1.88.0 (Multi-architecture Zip):

    ```shell
    # Find all available releases at: https://github.com/GoogleCloudPlatform/cluster-toolkit/releases
    # Set the desired version TAG (e.g., v1.85.0)
    TAG=vX.Y.Z
    # Set your OS (linux or mac) and Architecture (amd64 or arm64)
    OS="linux"
    ARCH="amd64"
    # Download and extract the platform-specific bundle
    curl -LO https://github.com/GoogleCloudPlatform/cluster-toolkit/releases/download/${TAG}/gcluster_bundle_${OS}_${ARCH}.zip
    unzip gcluster_bundle_${OS}_${ARCH}.zip -d cluster-toolkit/
    cd cluster-toolkit
    ```

2. Verify the Installation:

    ```shell
    ./gcluster --version
    ./gcluster --help
    ```

### Building from Source

If you prefer to build the `gcluster` binary from source,
you can use the following commands:

```bash
git clone https://github.com/GoogleCloudPlatform/cluster-toolkit
cd cluster-toolkit
make
./gcluster --version
./gcluster --help
```

Note: You must [install dependencies](https://cloud.google.com/cluster-toolkit/docs/setup/install-dependencies) (such as Go and Terraform) before building, otherwise the `make` command will fail.

## Prerequisites

Before deploying your first cluster, ensure the following are configured in your Google Cloud project.

### Enable APIs

Several APIs must be enabled to deploy your cluster. While Terraform will identify missing APIs during `terraform apply`, enabling them upfront saves time. Required APIs typically include:
- Compute Engine API
- Filestore API
- Cloud Storage API
- Service Usage API

See the [Google Cloud Docs](https://cloud.google.com/cluster-toolkit/docs/setup/configure-environment#enable-apis) for detailed instructions.

### Quotas

HPC and AI workloads often require significant resources. You might need to request additional quota to deploy your cluster. For more information, see [Request additional quotas](https://cloud.google.com/cluster-toolkit/docs/setup/hpc-blueprint#request-quota).

### GCP Credentials

Terraform can provide credentials for authenticating to Google Cloud in several ways. We recommend using `gcloud` on your workstation or using service accounts attached to cloud environments.

**Warning**: We do not recommend downloading or using service account JSON keys. These keys are long-lived credentials that pose a significant security risk if leaked. Instead, use short-lived credentials via Application Default Credentials (ADC).

On your local terminal or Cloud Workstations terminal, generate credentials associated with your Google Cloud account:

```bash
gcloud auth application-default login
```

Follow the prompts in your browser to authenticate. You will be provided a token to copy and paste back into your terminal to complete the process. Once finished, Terraform will automatically use these "Application Default Credentials."

If you receive "quota project" errors, set the quota project to your current project ID:

```bash
gcloud auth application-default set-quota-project ${PROJECT_ID}
```

### Telemetry and Privacy Notice

To help improve Cluster Toolkit, feature usage statistics are collected and sent to Google. You can opt-out at any time by executing the following command:

```bash
./gcluster telemetry off
```

Cluster Toolkit telemetry overall is handled in accordance with the [Google Privacy Policy](https://policies.google.com/privacy). When you use Cluster Toolkit to interact with or utilize GCP Services, your information is handled in accordance with the [Google Cloud Privacy Notice](https://cloud.google.com/terms/cloud-privacy-notice).

## Cluster Creation and Management

After installing `gcluster`, you can deploy, manage, and destroy infrastructure using Cluster Blueprints:

1. **Create Deployment Folder**: Process a blueprint YAML file to generate deployment files:

   ```bash
   ./gcluster create examples/hpc-slurm.yaml
   ```

2. **Deploy Cluster**: Provision infrastructure with Terraform:

   ```bash
   ./gcluster deploy hpc-slurm
   ```

3. **Destroy Cluster**: Clean up provisioned resources:

   ```bash
   ./gcluster destroy hpc-slurm
   ```

For detailed guides, blueprint syntax, and configuration options, see the [Google Cloud documentation site](https://cloud.google.com/cluster-toolkit/docs/overview).

## Job Submission (gcluster job submit)

The `gcluster job submit` command provides a unified interface to submit batch and distributed containerized workloads (JobSets) to your GKE clusters.

### Prerequisites for Job Submission

Ensure your environment is set up before submitting jobs:
- A deployed GKE cluster managed by Cluster Toolkit (with Kueue and JobSet enabled).
- `kubectl` and `gke-gcloud-auth-plugin` installed and authenticated (`gcloud auth login`).
- For on-the-fly builds (`--build-context`), set `export GCLUSTER_IMAGE_REPO=<repository-name>` to specify your Artifact Registry repository name (e.g., `export GCLUSTER_IMAGE_REPO=gcluster-repo`).

### Submitting a Job

**Submitting with a Pre-built Image**:

```bash
./gcluster job submit \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION_OR_ZONE> \
  --name my-job \
  --image us-docker.pkg.dev/samples/gke/hello-app:1.0 \
  --command "echo Hello from Cluster Toolkit" \
  --compute-type n2-standard-32
```

**Submitting with On-the-Fly Image Building**:

```bash
./gcluster job submit \
  --project <PROJECT_ID> \
  --cluster <CLUSTER_NAME> \
  --location <REGION_OR_ZONE> \
  --name my-python-job \
  --base-image python:3.11-slim \
  --build-context ./app_dir \
  --command "python main.py" \
  --compute-type n2-standard-32
```

### Managing Jobs

- **List Jobs**: Check workload status across the cluster:

  ```bash
  ./gcluster job list
  ```

- **View Job Logs**: View container stdout/stderr logs:

  ```bash
  ./gcluster job logs my-job
  ```

- **Cancel a Job**: Clean up a running workload:

  ```bash
  ./gcluster job cancel my-job
  ```

For complete step-by-step tutorials, advanced multi-slice topologies, storage mounts, and Kueue queue management, refer to the full [Gcluster Job Submission Guide](docs/gcluster_job_guide.md).

## VM Image support

### Slurm images

The Cluster Toolkit provides specialized modules for Slurm. **Note**: Slurm Terraform modules must be used with images specifically built for the versioned release of the module. To learn more about pre-built and custom Slurm images, see [Slurm VM Images](docs/vm-images.md#slurm-on-gcp).

### Standard images

The toolkit also supports standard OS images for general-purpose modules:
- HPC Rocky Linux 8
- Debian 11
- Ubuntu 22.04 LTS

For more details, see [VM Image Support](docs/vm-images.md).

## Blueprint validation

A **Cluster Blueprint** is the core configuration file (YAML) for your deployment. Cluster Toolkit includes **validator** functions that perform basic tests on the blueprint to ensure variables are valid and resources can be provisioned. See [Blueprint Validation](docs/blueprint-validation.md) for more details.

## Billing reports

To track the costs of your deployment, use the [Cloud Billing Reports](https://cloud.google.com/billing/docs/how-to/reports) page. You need a role with the `billing.accounts.getSpendingInformation` permission.

1. In the Google Cloud Console, go to **Billing**.
2. Select **Reports**.
3. In the **Filters** pane on the right, filter by label using the key `ghpc_deployment` or `ghpc_blueprint` and specify your deployment name.

## Troubleshooting

### Authentication
Ensure you have properly [set up Google Cloud credentials](#gcp-credentials).

### Slurm Clusters
See [Slurm Troubleshooting](docs/slurm-troubleshooting.md).

### Terraform Deployment
Common deployment failures:
- **Project Access**: Ensure your account has the necessary roles in the IAM section of the console.
- **Filestore resource limit**: If you see "System limit for internal resources has been reached," see the [Filestore troubleshooting guide](https://cloud.google.com/filestore/docs/troubleshooting#system_limit_for_internal_resources_has_been_reached_error_when_creating_an_instance) for the solution.

## Development

**Note for macOS users**: While macOS is supported for building and running the toolkit, it is not recommended for core development due to GNU-specific shell scripts. If developing on macOS, install GNU tools (e.g., `coreutils`, `findutils`) via Homebrew or Conda to avoid script failures.

### Setup

Install the following tools to ensure your changes pass validation:
- [pre-commit](https://pre-commit.com/)
- [TFLint](https://github.com/terraform-linters/tflint#installation) (requires version compatible with `.tflint.hcl`)
- [ShellCheck](https://github.com/koalaman/shellcheck#installing)

Additional development dependencies can be installed with a single command:

```bash
make install-dev-deps
pre-commit install
```

### Contributing

Please refer to [CONTRIBUTING.md](CONTRIBUTING.md).
