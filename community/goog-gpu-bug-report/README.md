# GCP GPU Bug Report Collector

![Platform: GCP](https://img.shields.io/badge/Platform-GCP-green.svg)

A universal tool to simplify the generation of GPU diagnostic logs (including
NVIDIA bug reports) on Google Compute Platform (GCP) GPU VMs and GKE nodes
across Container-Optimized OS (COS), Ubuntu, Debian, and RHEL-family guest
operating systems (such as Rocky Linux and CentOS Stream), on both x86
(`linux/amd64`) and Arm (`linux/arm64`) machines.

This script provides a simple and reliable one-command experience to collect
standard `nvidia-bug-report` logs alongside additional GPU topology, storage
I/O, RDMA, and NVLink/IMEX fabric diagnostics, bundled into a single `.tar.gz`
archive. For GPUs with **Blackwell** architectures and newer, it automatically
installs the
[NVIDIA MFT (Nvidia Firmware Tools)](https://docs.nvidia.com/networking/display/mftv4330)
to generate a more comprehensive report with deep hardware diagnostics.

--------------------------------------------------------------------------------

## 🤔 The Challenge: Getting GPU Bug Report on GCP across Linux OSes

When troubleshooting GPU issues, the first step is often to generate an
`nvidia-bug-report`. However, doing so on **Google Compute Platform (GCP) GPU
VMs and GKE nodes across different Linux operating systems** could be less
trivial:

* **Container-Optimized OS (COS)** is a minimal, security-hardened operating
    system from Google, designed specifically for running containers. By design,
    it does not include many standard packages or libraries that general-purpose
    debug tools often rely on and is designed to mainly execute userspace
    programs through containers. This stripped down nature therefore requires
    some additional efforts to collect and export a comprehensive GPU bug report
    on COS systems.
* **Different NVIDIA Driver Paths Across Linux Distributions**: COS installs
    the driver under `/var/lib/nvidia`, GKE under `/home/kubernetes/bin/nvidia`,
    and general-purpose Linux distributions under `/usr` alongside host system
    libraries. Giving a container access to the host's NVIDIA driver files
    without overriding the container's own libraries requires careful runtime or
    mount handling.
* **Need for Additional Fabric, Topology, and Storage Diagnostics**: On newer
    multi-node NVLink and RDMA GPU shapes, triaging issues requires more than
    `nvidia-bug-report.log.gz` alone—it also requires IMEX daemon status and
    logs, RDMA link state (`ibv_devinfo`), GPU/NIC topology (`nvidia-smi topo
    -m`), and disk I/O statistics (`iostat`).

### 🔬 Enhanced Bug Report for Blackwell & Newer GPUs

For newer GPU architectures like **NVIDIA Blackwell**, a standard NVIDIA bug
report, while useful, may not be sufficient for diagnosing complex
hardware-level issues, especially those related to
[NVLink](https://www.nvidia.com/en-us/data-center/nvlink/). A truly
comprehensive report requires deeper diagnostic data.

This is where the NVIDIA MFT suite becomes essential. You do not need to
interact with MFT directly; instead, the `nvidia-bug-report` script is designed
to automatically leverage the MFT utilities if they are present on the system.
By doing so, it can generate a far more comprehensive GPU bug report for
diagnostics.

When available, MFT allows the bug report to include critical, low-level
hardware data such as:

* The physical layer status of NVLink connections.
* Internal GPU register values and configuration data.
* Raw diagnostic segments generated directly by the firmware.

However, setting up MFT across operating systems is a cumbersome process:

1. **Kernel Module Handling**:
   * **On COS**: A user must first locate and download the specific,
     **COS-signed** MFT kernel modules (`mst_pci.ko` and `mst_pciconf.ko`)
     that perfectly correspond to their exact COS image version. Only a
     signed, version-matched module can be loaded into the COS kernel.
   * **On Ubuntu, Debian, and RHEL-family OSes**: No pre-compiled MFT modules
     exist, so the kernel modules must be built from source via DKMS against
     the host's running kernel headers—or fall back to driverless userspace
     mode (`--without-kernel`) when headers are absent or module loading is
     restricted (driverless mode still collects the GPU NVLink diagnostics,
     such as `mlxlink` and `resourcedump`, via the NVIDIA driver).
2. **Userspace Program and Containerization**: To provide a consistent
    environment across operating systems (and following the COS design
    philosophy where applications run in containers), the MFT userspace programs
    and diagnostic tools run inside a container and must be compatible with the
    kernel modules. The container must also have access to the host's NVIDIA
    driver userspace, PID/UTS namespaces, systemd runtime, and system logs so
    that `nvidia-bug-report.sh` captures accurate host state rather than
    container state.
3. **Execution and Export**: The bug report generation and supplementary
    diagnostic commands must be triggered from within this container, and all
    resulting logs must be packaged into a single archive and exported to the
    host VM or a GCS bucket.

## 💡 Our Solution: A Smart, All-in-One Collector

This script eliminates all of the aforementioned complexity. It acts as a
universal collector that simplifies bug report generation for all users on GCP
across COS, Ubuntu, Debian, and RHEL-family operating systems (on both GCE VMs
and GKE):

* **For all supported GPUs**, it automates the steps needed to generate a
    standard `nvidia-bug-report` along with GPU/NIC topology, RDMA, IMEX, and
    disk I/O diagnostics, bundled into a single `.tar.gz` archive.
* **For Blackwell and newer GPU architectures**, it automatically detects the
    hardware and handles the entire MFT setup process in the background (loading
    signed modules on COS, building via DKMS on other OSes, or falling back to
    driverless MFT) and then generates a more comprehensive bug report.

This transforms the NVIDIA GPU bug report generation task on GCP into a single
command (`run-goog-gpu-bug-report.sh` on GCE VMs, or `bug-report-pod.yaml` on
GKE).

### ✨ Key Features

* **Universal Multi-OS & Multi-Arch Collector**: A single, simple command to
    generate a GPU triage bundle on GCE VMs and GKE nodes running COS, Ubuntu,
    Debian, or RHEL-family operating systems across `linux/amd64` and
    `linux/arm64`.
* **Automatic NVIDIA Userspace Access (Container Toolkit + Host-Mount
    Fallback)**: The launcher script (`run-goog-gpu-bug-report.sh`) first uses
    the NVIDIA Container Toolkit (`--runtime=nvidia` when `nvidia-ctk >= 1.18.0`
    is installed and its runtime mode is `auto`, `jit-cdi`, or `cdi`) and
    automatically falls back to bind-mounting the host's driver directories
    (`/var/lib/nvidia` on COS, `/usr` at `/host_usr` on other OSes) if the
    toolkit is unavailable or incomplete.
* **Automatic MFT Enhancement with Driverless Fallback**: For Blackwell and
    newer GPUs, the script automatically installs and configures the NVIDIA MFT
    suite—loading COS-signed modules on COS or building via DKMS on other OSes,
    and gracefully falling back to driverless MFT (`--without-kernel`) if kernel
    modules cannot be loaded (configurable via
    `--mft_kernel_modules={auto,required,never}`).
* **Extended Fabric, Topology, and Storage Diagnostics**: Collects key
    additional logs (`nvidia-smi topo -m`, `iostat -xt`, `ibv_devinfo`, and
    `nvidia-imex-ctl -N` + `nvidia-imex.log`) alongside
    `nvidia-bug-report.log.gz`.
* **Single Archive & Optional GCS Upload**: Bundles all diagnostic outputs
    into one `.tar.gz` archive locally and optionally uploads it directly to a
    Google Cloud Storage bucket for easy sharing and analysis.

## 📋 Prerequisites

Before running the script, ensure you have:

1. A Google Compute Engine (GCE) GPU VM instance or a GKE node running
   **Container-Optimized OS (COS)**, **Ubuntu**, **Debian**, or a **RHEL-family
   OS** (such as Rocky Linux and CentOS Stream).

2. The GPU driver is installed on the GCE VM instance.

   * **On COS**, please refer to
     [COS's official documentation page](https://cloud.google.com/container-optimized-os/docs/how-to/run-gpus#install)
     for more detail.
   * **On Ubuntu, Debian, and RHEL-family OSes**, please refer to
     [Install GPU drivers on VMs](https://cloud.google.com/compute/docs/gpus/install-drivers-gpu).
   * Sample commands to install the GPU driver on COS and verify the
     installation:
   * ***NOTE:*** For GKE the driver is already installed. Unless opted for
     [manual driver installation](https://cloud.google.com/kubernetes-engine/docs/how-to/gpus#create-gpu-pool-auto-drivers)

   ```bash
   # Install NVIDIA GPU Driver
   sudo cos-extensions install gpu -- --version=latest

   # Make the driver installation path executable by re-mounting it.
   sudo mount --bind /var/lib/nvidia /var/lib/nvidia
   sudo mount -o remount,exec /var/lib/nvidia

   # Display all GPUs
   /var/lib/nvidia/bin/nvidia-smi
   ```

3. Configure Docker to use your Artifact Registry credentials when interacting
   with Artifact Registry.

   * Please refer to
     [Artifact Registry's authentication page](https://cloud.google.com/artifact-registry/docs/docker/authentication)
     for more detail.
   * **On non-COS Linux distributions** (Ubuntu, Debian, Rocky Linux, CentOS
     Stream), non-root users are not in the `docker` group by default. Add your
     user to the `docker` group so Docker runs without `sudo` (or, if you prefer
     running `docker` commands with `sudo`, configure credentials with `sudo` as
     well since `sudo` reads `/root/.docker/config.json` instead of
     `$HOME/.docker/config.json`):

   ```bash
   sudo usermod -aG docker $USER && newgrp docker
   ```

   * Sample commands to configure the Docker credential:

   ```bash
   ARTIFACT_REGISTRIES="us-central1-docker.pkg.dev"
   # If using standalone credential helper:
   docker-credential-gcr configure-docker --registries=${ARTIFACT_REGISTRIES?}
   # If using gcloud CLI credential helper:
   gcloud auth configure-docker ${ARTIFACT_REGISTRIES?}
   ```

4. [Optional, Recommended for non-COS OS] NVIDIA Container Toolkit (`1.18.0` or
   newer) installed and configured for Docker so the launcher can use
   `--runtime=nvidia` before falling back to host directory mounts. The
   toolkit's runtime mode (`nvidia-container-runtime.mode` in
   `/etc/nvidia-container-runtime/config.toml`) must be `auto` (the default),
   `jit-cdi`, or `cdi`.

   * Please refer to
     [NVIDIA Container Toolkit's installation guide](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html)
     for more detail.
   * Sample commands to configure the NVIDIA Container Toolkit runtime for
     Docker once installed:

   ```bash
   sudo nvidia-ctk runtime configure --runtime=docker
   sudo systemctl restart docker
   ```

5. [Optional] If you would like to export the bug report to GCS, the VM's
   service account must have *at least* Storage Object Creator
   (`roles/storage.objectCreator`) and Storage Bucket Viewer
   (`roles/storage.bucketViewer`) permissions for the target bucket.

   * Our script would attempt to create the specified GCS bucket when the
     specified bucket does not exist in the project. If you would like to
     leverage this feature, then your service account needs to have the
     **Storage Admin (`roles/storage.admin`)** role.

   * Sample commands to grant storage admin permission to your project's
     service account:

   ```bash
   PROJECT=... # your project id
   gcloud projects add-iam-policy-binding ${PROJECT?} \
   --member="serviceAccount:$(gcloud iam service-accounts list --project=${PROJECT?} \
   --filter="email~'-compute@developer.gserviceaccount.com'" --format="value(email)")" \
   --role='roles/storage.admin'
   ```

## 🚀 Quick Start

This tool is designed to be run as a Docker container, either via the launcher
script `run-goog-gpu-bug-report.sh` (recommended) or via a direct `docker run`
command.

### To run in a GCE VM instance

#### Option 1: Using the launcher script (Recommended)

Copy `run-goog-gpu-bug-report.sh` to the VM and run it with `bash` (using `bash`
avoids `noexec` mount restrictions on `/home` and `/tmp` on COS):

```bash
bash run-goog-gpu-bug-report.sh -- --gcs_bucket=${GCS_BUCKET}
```

Note: Exporting the final bug reports to a GCS bucket is optional. If you do not
intend to export it elsewhere, you may remove `-- --gcs_bucket=${GCS_BUCKET}` at
the end.

The launcher first tries running with `--runtime=nvidia` (if
`nvidia-ctk >= 1.18.0` is available and its runtime mode is `auto`, `jit-cdi`,
or `cdi`) and automatically falls back to mounting the host's driver directories
(`/var/lib/nvidia` on COS, or `/usr:/host_usr:ro` on other Linux distributions)
if the toolkit runtime is unavailable or fails.

For Blackwell and newer GPUs, installing the MFT kernel modules (`mst_pci` and
`mst_pciconf`) is recommended and enabled by default
(`--mft_kernel_modules=auto`): on COS, the collector downloads and loads
pre-built, COS-signed kernel modules; on other operating systems, it builds the
modules via DKMS and loads them (falling back to driverless MFT if loading
fails). If you do not want to build or load MFT kernel modules on the host, pass
`--mft_kernel_modules=never`:

```bash
bash run-goog-gpu-bug-report.sh -- --mft_kernel_modules=never --gcs_bucket=${GCS_BUCKET}
```

Launcher options and collector flags:

<!-- mdformat off(GitHub does not render multi-line table rows) -->

| Option / Flag                   | Description                                                                                                |
| :------------------------------ | :--------------------------------------------------------------------------------------------------------- |
| `--image=IMAGE`                 | Launcher option: overrides the collector container image (defaults to `DEFAULT_IMAGE` in the script).      |
| `-- FLAGS...`                   | Passes all following flags to `goog-gpu-bug-report.py`.                                                    |
| `--gcs_bucket=BUCKET`           | Collector flag: uploads the final `.tar.gz` bug report archive to this GCS bucket.                         |
| `--mft_kernel_modules=auto`     | Collector flag (default): tries to load MFT kernel modules (signed on COS, DKMS on other OSes) and falls back to driverless MFT. |
| `--mft_kernel_modules=required` | Collector flag: fails the run if the MFT kernel modules cannot be loaded.                                  |
| `--mft_kernel_modules=never`    | Collector flag: unloads and blocks MFT kernel modules on the host and runs MFT in driverless mode.         |

<!-- mdformat on -->

#### Option 2: Direct `docker run` command

If running `docker run` directly without `run-goog-gpu-bug-report.sh`, you can
run the container either with the NVIDIA Container Toolkit or with host
directory mounts:

* **With the NVIDIA Container Toolkit (`--runtime=nvidia`)**: Uses the toolkit
    runtime (`1.18.0` or newer, in runtime mode `auto`, `jit-cdi`, or `cdi`) to
    inject the host's NVIDIA driver libraries and GPU devices, and mounts the
    host diagnostic scripts (`nvidia-bug-report.sh`,
    `nvidia-networking-bug-report.py`, and `nv-fabricmanager`) alongside host
    system logs:

    ```bash
    docker run --rm \
      --name goog-gpu-bug-report \
      --pull=always \
      --privileged --network=host --uts=host --pid=host \
      --runtime=nvidia \
      -e NVIDIA_VISIBLE_DEVICES=all \
      -e SYSTEMD_IGNORE_CHROOT=1 \
      --volume /tmp:/tmp \
      --volume /dev:/dev \
      --volume /run:/run \
      $(PATH="$PATH:/var/lib/nvidia/bin"; for t in nvidia-bug-report.sh nvidia-networking-bug-report.py nv-fabricmanager; do p=$(command -v "$t") && echo "--volume $p:$p:ro"; done) \
      $([ -d /var/log ]               && echo "--volume /var/log:/var/log:ro") \
      $([ -d /lib/modules ]           && echo "--volume /lib/modules:/lib/modules") \
      $([ -d /usr/src ]               && echo "--volume /usr/src:/usr/src") \
      $([ -f /etc/os-release ]        && echo "--volume /etc/os-release:/etc/os-release:ro") \
      $([ -f /etc/lsb-release ]       && echo "--volume /etc/lsb-release:/etc/lsb-release:ro") \
      $([ -f /etc/machine-id ]        && echo "--volume /etc/machine-id:/etc/machine-id:ro") \
      $([ -f /etc/issue ]             && echo "--volume /etc/issue:/etc/issue:ro") \
      $([ -d /etc/nvidia-imex ]       && echo "--volume /etc/nvidia-imex:/etc/nvidia-imex:ro") \
      us-central1-docker.pkg.dev/gce-ai-infra/goog-gpu-bug-report-repo/goog-gpu-bug-report:latest \
      --gcs_bucket=${GCS_BUCKET}
    ```

* **Host-Mount Mode (without NVIDIA Container Toolkit)**: Mounts the host's
    driver directories (`/var/lib/nvidia` on COS, `/usr` at `/host_usr` on
    non-COS Linux distributions) along with host OS details, systemd service
    status, and journal logs:

    ```bash
    docker run --rm \
      --name goog-gpu-bug-report \
      --pull=always \
      --privileged --network=host --uts=host --pid=host \
      -e SYSTEMD_IGNORE_CHROOT=1 \
      --volume /tmp:/tmp \
      --volume /dev:/dev \
      --volume /run:/run \
      --volume /usr:/host_usr:ro \
      $([ -d /var/lib/nvidia ]        && echo "--volume /var/lib/nvidia:/var/lib/nvidia:ro") \
      $([ -d /var/log ]               && echo "--volume /var/log:/var/log:ro") \
      $([ -d /lib/modules ]           && echo "--volume /lib/modules:/lib/modules") \
      $([ -d /usr/src ]               && echo "--volume /usr/src:/usr/src") \
      $([ -f /etc/os-release ]        && echo "--volume /etc/os-release:/etc/os-release:ro") \
      $([ -f /etc/lsb-release ]       && echo "--volume /etc/lsb-release:/etc/lsb-release:ro") \
      $([ -f /etc/machine-id ]        && echo "--volume /etc/machine-id:/etc/machine-id:ro") \
      $([ -f /etc/issue ]             && echo "--volume /etc/issue:/etc/issue:ro") \
      $([ -d /etc/nvidia-imex ]       && echo "--volume /etc/nvidia-imex:/etc/nvidia-imex:ro") \
      us-central1-docker.pkg.dev/gce-ai-infra/goog-gpu-bug-report-repo/goog-gpu-bug-report:latest \
      --gcs_bucket=${GCS_BUCKET}
    ```

### To run in a GKE cluster

Note: Update the below fields in the pod manifest `./bug-report-pod.yaml`

<!-- mdformat off(GitHub does not render multi-line table rows) -->

| Fields | Description | Example |
|:--------:|:--------:|:--------:|
|  `<gke-node-name>`  |   GKE node which we want to target  |  gke-a4-2-a4-highgpu-8g-a4-p-b99cb17a-xrre  |
|  `<number_of_gpus>`  |  Total number of GPUs on the targeted GPU node  |  A4 - 8, A4X - 4  |

<!-- mdformat on -->

Note: Exporting the final bug reports to a GCS bucket is optional. If you do not
intend to export it elsewhere, you may remove the `--gcs_bucket=${GCS_BUCKET}`
at the end.

```bash
kubectl apply -f cluster-toolkit/community/goog-gpu-bug-report/bug-report-pod.yaml
```

### 📝 Example Output

```bash
I1019 23:24:51.010709 274063027707936 goog-gpu-bug-report.py:1486] Bug report logs are available locally at: /tmp/goog_gpu_bug_reports/utc_2026_10_19_23_24_23/vm_id_868647182909775754
I1019 23:24:51.010743 274063027707936 goog-gpu-bug-report.py:1491] Master bug report archive created at: /tmp/goog_gpu_bug_reports/utc_2026_10_19_23_24_23/vm_id_868647182909775754/goog_gpu_bug_report_868647182909775754_utc_2026_10_19_23_24_23.tar.gz
I1019 23:24:51.275231 274063027707936 goog-gpu-bug-report.py:1508] Bug report master archive is available at: https://console.cloud.google.com/storage/browser/my-nv-bug-reports/bug_report/utc_2026_10_19_23_24_23/vm_id_868647182909775754
```

The output directory contains `instance_info.txt`, `nvidia-bug-report.log.gz`,
the five supplementary diagnostic files, and the `.tar.gz` archive (which
bundles all of them into a single file for GCS upload):

```bash
$ ls -la /tmp/goog_gpu_bug_reports/utc_2026_10_19_23_24_23/vm_id_868647182909775754
total 16008
drwxr-xr-x 2 root root    4096 Oct 19 23:24 .
drwxr-xr-x 3 root root    4096 Oct 19 23:24 ..
-rw-r--r-- 1 root root 7574589 Oct 19 23:24 goog_gpu_bug_report_868647182909775754_utc_2026_10_19_23_24_23.tar.gz
-rw-r--r-- 1 root root   12272 Oct 19 23:24 ibv_devinfo.txt
-rw-r--r-- 1 root root     800 Oct 19 23:24 imex_status.txt
-rw-r--r-- 1 root root     379 Oct 19 23:24 instance_info.txt
-rw-r--r-- 1 root root    2604 Oct 19 23:24 iostat.txt
-rw-r--r-- 1 root root 8767379 Oct 19 23:24 nvidia-bug-report.log.gz
-rw-r--r-- 1 root root    2340 Oct 19 23:24 nvidia_imex.log
-rw-r--r-- 1 root root    1093 Oct 19 23:24 nvidia_smi_topo.txt
```

<!-- mdformat off(GitHub does not render multi-line table rows) -->

| File | Description |
| :--- | :--- |
| `instance_info.txt` | Basic information about the GCE instance, MFT kernel module status, and how the container accessed the NVIDIA driver userspace. |
| `nvidia-bug-report.log.gz` | Generated by `nvidia-bug-report.sh`, containing GPU states, PCI tree topology, system `dmesg`/journal logs, and MFT diagnostics on Blackwell+ GPUs. |
| `nvidia_smi_topo.txt` | GPU and NIC interconnect topology matrix from `nvidia-smi topo -m`. |
| `iostat.txt` | Extended disk and storage I/O statistics from `iostat -xt`. |
| `ibv_devinfo.txt` | RDMA device status and port attributes from `ibv_devinfo` (collected on machine types with NVIDIA ConnectX NICs, such as A3 Ultra, A4, and A4X). |
| `imex_status.txt`, `nvidia_imex.log` | NVIDIA IMEX domain status (`nvidia-imex-ctl -N`) and daemon log (collected on multi-node NVLink machine types such as A4X). |
| `goog_gpu_bug_report_<vm_id>_<timestamp>.tar.gz` | Single archive containing all of the files above; uploaded to GCS when `--gcs_bucket` is specified. |

<!-- mdformat on -->

`instance_info.txt` holds the basic information about the GCE instance, eg. the
project id, instance id, machine type, MFT module status, and driver userspace
access mode:

```bash
GCE Instance Info:
    Project ID: my-gpu-project
    Instance ID: 868647182909775754
    Image: ubuntu-2204-jammy-arm64-v20261016
    Zone: us-central1-a
    Machine Type: a4x-highgpu-4g
    Architecture: Architecture.ARM
    MST Version: mst, mft 4.33.0-169, built on Jul 24 2025, 16:49:49. Git SHA Hash: N/A
    MFT Kernel Modules Loaded: True
    NVIDIA Driver Userspace: nct_runtime_cdi
```

* If you are running with GPU architectures supported by the NVIDIA MFT (eg.
    B200), you may also validate that the GPU NVLink information are also being
    recorded. You can easily validate it through searching for the keyword
    `Starting GPU MST dump..` or `Starting GPU NetIR dump` in the log file
    (passing `-a` to `grep` because the log contains binary data):

```bash
$ zcat /tmp/goog_gpu_bug_reports/utc_2026_10_18_09_21_29/vm_id_8220847375056493254/nvidia-bug-report.log.gz | grep -a -m 1 -A 30 -E "Starting GPU (MST|NetIR) dump"
Starting GPU MST dump.../dev/mst/netir10497_00.cc.00_gpu7
____________________________________________
/usr/bin/mlxlink -d /dev/mst/netir10497_00.cc.00_gpu7 --amber_collect /tmp/mlx96.csv > /tmp/mlx96.info 2>&1

Operational Info
----------------
State                              : Active
Physical state                     : N/A
Speed                              : NVLink-XDR
Width                              : 2x
FEC                                : Interleaved_Standard_RS_FEC_PLR - (544,514)
Loopback Mode                      : No Loopback
Auto Negotiation                   : ON

Supported Info
--------------
Enabled Link Speed                 : 0x00000100 (XDR)
Supported Cable Speed              : N/A

Troubleshooting Info
--------------------
Status Opcode                      : 0
Group Opcode                       : N/A
Recommendation                     : No issue was observed

Tool Information
----------------
Firmware Version                   : 36.2014.1676
amBER Version                      : 4.8
MFT Version                        : mft 4.33.0-169

```

## 🛠️ Developer Guide: Modifying and Releasing Your Own Image

This section is for developers who wish to customize the script's behavior or
release their own version of the container image to a private Google Artifact
Registry.

### 1. Modifying the Code

* The core logic for generating the bug report are located in the `app/`
    directory, with the main entry point being `goog-gpu-bug-report.py`.
* The image and all relevant dependencies to run the Python file above are
    defined in the `Dockerfile`.
* The host launcher script for GCE VMs is `run-goog-gpu-bug-report.sh`, and
    the Kubernetes pod manifest for GKE is `bug-report-pod.yaml`.

### 2. Building and Pushing to Artifact Registry

We provide a convenient shell script to build and push your customized image to
your own Artifact Registry (building a multi-arch manifest for `linux/amd64` and
`linux/arm64` via `docker buildx`).

You can do so by invoking the `build-and-push-goog-gpu-bug-report.sh` script
with the following parameters:

<!-- mdformat off(GitHub does not render multi-line table rows) -->

| Flag | Description                                                     | Required |
| :--- | :-------------------------------------------------------------- | :------- |
| `-p` | Your Google Cloud Project ID.                                   | **Yes**  |
| `-r` | The name of your Artifact Registry repository.                  | **Yes**  |
| `-i` | The name for your image.                                        | **Yes**  |
| `-l` | The region of your Artifact Registry. Defaults to `us-central1` | No       |
| `-h` | Display the help message.                                       | No       |

<!-- mdformat on -->

Sample command:

```bash
bash build-and-push-goog-gpu-bug-report.sh \
    -p ${PROJECT?} \
    -r ${ARTIFACT_REPO?} \
    -i "custom-bug-report-collector" \
    -l "us-east1"
```

To run your custom image via the launcher script, pass `--image`:

```bash
bash run-goog-gpu-bug-report.sh \
    --image=us-east1-docker.pkg.dev/${PROJECT?}/${ARTIFACT_REPO?}/custom-bug-report-collector:YYYY-MM-DD \
    -- --gcs_bucket=${GCS_BUCKET}
```
