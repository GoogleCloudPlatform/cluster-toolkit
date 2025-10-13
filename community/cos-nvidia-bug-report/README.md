# GCP COS NVIDIA Bug Report Collector

![Platform: GCP/COS](https://img.shields.io/badge/Platform-GCP%2FCOS-green.svg)

A universal tool to simplify the generation of NVIDIA bug reports on Google
Compute Platform (GCP) VMs that use the Container-Optimized OS (COS) guest
operating system.

This script provides a simple and reliable one-command experience to collect
standard `nvidia-bug-report` logs. For GPUs with **Blackwell** architectures and
newer, it automatically installs the
[NVIDIA MFT (Nvidia Firmware Tools)](https://docs.nvidia.com/networking/display/mftv4320)
to generate a more comprehensive report with deep hardware diagnostics.

--------------------------------------------------------------------------------

## 🤔 The Challenge: Getting GPU Bug Report on GCP with COS

When troubleshooting GPU issues, the first step is often to generate an
`nvidia-bug-report`. However, doing so on a **Google Compute Platform (GCP)
that uses Container-Optimized OS (COS) as its guest operating
system** could be a less trivial.

COS is a minimal, security-hardened operating system from Google, designed
specifically for running containers. By design, it does not include many
standard packages or libraries that general-purpose debug tools often rely on
and is design to mainly execute userspace programs through containers. This
stripped down nature therefore requires some additional efforts to collect and
export a comprehensive GPU bug report on COS systems.

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

However, setting up MFT is a cumbersome process on COS:

1. **Kernel Module Handling**: A user must first locate and download the
    specific, **COS-signed** MFT kernel module that perfectly corresponds to
    their exact COS image version. Only a signed, version-matched module can be
    loaded into the COS kernel.
2. **Userspace Program and Containerization**: Following the COS design
    philosophy, all applications should run in containers. This means the user
    must create a custom container that includes the MFT userspace programs,
    which also must be compatible with the kernel module.
3. **Execution and Export**: The bug report generation must be triggered from
    within this custom container. Afterward, a mechanism is needed to export the
    final log file from the container out to the host VM or a GCS bucket.

## 💡 Our Solution: A Smart, All-in-One Collector

This script eliminates all of the aforementioned complexity. It acts as a
universal collector that simplifies bug report generation for all users on GCP
with COS.

* **For all supported GPUs**, it automates the steps needed to generate a
    standard `nvidia-bug-report`.
* **For Blackwell and newer GPU architectures**, it automatically detects the
    hardware and handles the entire MFT setup process in the background and then
    generates a more comprehensive bug report.

This transforms the NVIDIA GPU bug report generation task on COS into a single
docker command.

### ✨ Key Features

* **Universal Collector for GCP COS**: A single, simple command to generate an
    `nvidia-bug-report` on any supported GCP machine with the COS guest OS.
* **Automatic MFT Enhancement**: For Blackwell and newer GPUs, the script
    automatically installs and configures the NVIDIA MFT suite to unlock deeper,
    more comprehensive hardware diagnostics.
* **Optional GCS Upload**: Directly uploads the final report to a Google Cloud
    Storage bucket for easy sharing and analysis.

## 📋 Prerequisites

Before running the script, ensure you have:

1. A Google Compute Engine (GCE) GPU VM instance or a GKE node with **Container-Optimized OS
    (COS)** as its guest operating system.

2. The GPU driver is installed on the GCE VM instance.
   * Please refer to
        [COS's official documentation page](\(https://cloud.google.com/container-optimized-os/docs/how-to/run-gpus#install\))
        for more detail.
   * Sample commands to install the GPU driver and verify the installation:
   * ***NOTE:*** For GKE the driver is already installed. Unless opted for [manual driver installation](https://cloud.google.com/kubernetes-engine/docs/how-to/gpus#create-gpu-pool-auto-drivers)

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
   * Sample commands to configure the docker credential:

    ```bash
    ARTIFACT_REGISTRIES="us-central1-docker.pkg.dev"
    docker-credential-gcr configure-docker --registries=${ARTIFACT_REGISTRIES?}
    ```

4. [Optional] If you would like to export the bug report to GCS, the VM's
    service account must have *at least* Storage Object Creator
    (`roles/storage.objectCreator`) permissions for the target bucket.

   * Our script would attempt to create the specified GCS bucket when the
        specified bucket does not exist in the project. If you would like to
        leverage this feature, then your service account needs to have the
        **Storage Admin (`roles/storage.admin`)** role.

   * Sample commands to grant storage admin permission to your project's
        service account:

    ```bash
    PROJECT=... # your project id
    gcloud projects add-iam-policy-binding ${PROJECT?} \
    --member="serviceAccount:$(staging_gcloud iam service-accounts list --project=${PROJECT?} \
    --filter="email~'-compute@developer.gserviceaccount.com'" --format="value(email)")" \
    --role='roles/storage.admin'
    ```

## 🚀 Quick Start

This tool is designed to be run as a Docker container. The primary method of use
is a single docker run command.

### To run in a GCE VM instance

Sample command to run on a GCE VM with 8 GPUs:

Note: If you have a different number of GPUs on your system, you may need to
adjust the `--device /dev/nvidia<gpu_num>:/dev/nvidia<gpu_num>` in the docker
command accordingly.

Note: Exporting the final bug reports to a GCS bucket is optional. If you do not
intend to export it elsewhere, you may remove the `--gcs_bucket=${GCS_BUCKET}`
at the end.

```bash
docker run \
  --name gce-cos-bug-report \
  --pull=always \
  --privileged \
  --network=host \
  --volume /etc:/etc_host \
  --volume /tmp:/tmp \
  --volume /var/lib/nvidia:/usr/local/nvidia \
  $(find /dev -regextype posix-extended -regex '/dev/nvidia[0-9]+' | xargs -I {} echo '--device={}:{}')  \
  --device /dev/nvidia-uvm:/dev/nvidia-uvm \
  --device /dev/nvidiactl:/dev/nvidiactl \
us-central1-docker.pkg.dev/gce-ai-infra/gce-cos-nvidia-bug-report-repo/gce-cos-nvidia-bug-report:latest \
--gcs_bucket=${GCS_BUCKET}
```

### To run in a GKE cluster
Note: Update the below fields in the pod manifest `./bug-report-pod.yaml`

| Fields | Description | Example |
|:--------:|:--------:|:--------:|
|  `<gke-node-name>`  |   GKE node which we want to target  |  gke-a4-2-a4-highgpu-8g-a4-p-b99cb17a-xrre  |
|  `<number_of_gpus>`  |  Total number of GPUs on the targeted GPU node  |  A4 - 8, A4X - 4  |

Note: Exporting the final bug reports to a GCS bucket is optional. If you do not
intend to export it elsewhere, you may remove the `--gcs_bucket=${GCS_BUCKET}`
at the end.

```bash
kubectl apply -f cluster-toolkit/community/cos-nvidia-bug-report/bug-report-pod.yaml
```

### 📝 Example Output

```bash
I0624 21:37:25.424124 137683091463040 gce-cos-nvidia-bug-report.py:817] Bug report logs are available locally at: /tmp/nvidia_bug_reports/utc_2025_06_24_21_35_44/vm_id_2858600067712410553
I0624 21:37:25.794605 137683091463040 gce-cos-nvidia-bug-report.py:312] Bucket [my-nv-bug-reports] already exists in the project.
I0624 21:37:26.308939 137683091463040 gce-cos-nvidia-bug-report.py:834] Bug report logs are available at: https://pantheon.corp.google.com/storage/browser/my-nv-bug-reports/bug_report/utc_2025_06_24_21_35_44/vm_id_2858600067712410553
```

There will be two files getting generated as final outputs: i.e.
`instance_info.txt` and `nvidia-bug-report.log.gz`.

```bash
$ ls -la /tmp/nvidia_bug_reports/utc_2025_06_24_21_35_44/vm_id_2858600067712410553
total 19576
drwxr-xr-x 2 root root       80 Jun 24 21:37 .
drwxr-xr-x 3 root root       60 Jun 24 21:35 ..
-rw-r--r-- 1 root root      293 Jun 24 21:37 instance_info.txt
-rw-r--r-- 1 root root 20038087 Jun 24 21:37 nvidia-bug-report.log.gz
```

The first file holds the basic information about the GCE instance, eg. the
project id, instance id, machine type etc.

```bash
GCE Instance Info:
    Project ID: gpu-test-project-staging
    Instance ID: 8203180632673949960
    Image: cos-121-18867-90-38
    Zone: us-central1-staginga
    Machine Type: a4-highgpu-8g
    Architecture: Architecture.X86
    MST Version: mst, mft 4.32.0-120, built on Apr 30 2025, 09:17:51. Git SHA Hash: N/A
```

The second file, generated by `nvidia-bug-report.sh`, contains more information
on the GPU devices and the system in general, including the GPU states, PCI tree
topology, system dmesg, etc.

* If you are running with GPU architectures supported by the NVIDIA MFT (eg.
    B200), you may also validate that the GPU NVLink information are also being
    recorded. You can easily validate it through searching for the keyword
    `Starting GPU MST dump..` in the unzipped log file:

```bash
$ sudo gunzip /tmp/nvidia_bug_reports/utc_2025_06_25_21_21_23/vm_id_8220847375056493254/nvidia-bug-report.log.gz
$ grep -m 1 -A 30 "Starting GPU MST dump.." /tmp/nvidia_bug_reports/utc_2025_06_25_21_21_23/vm_id_8220847375056493254/nvidia-bug-report.log
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
MFT Version                        : mft 4.32.0-120

```

## 🛠️ Developer Guide: Modifying and Releasing Your Own Image

This section is for developers who wish to customize the script's behavior or
release their own version of the container image to a private Google Artifact
Registry.

### 1. Modifying the Code

* The core logic for generating the bug report are located in the `app/`
    directory, with the main entry point being `gce-cos-nvidia-bug-report.py`.
* The image and all relevant dependencies to run the Python file above are
    defined in the `Dockerfile`.

### 2. Building and Pushing to Artifact Registry

We provide a convenient shell script to build and push your customized image to
your own Artifact Registry.

You can do so by invoking the `build-and-push-cos-nvidia-bug-report.sh`
script with the following parameters:

| Flag | Description                                                     | Required |
| :--- | :-------------------------------------------------------------- | :------- |
| `-p` | Your Google Cloud Project ID.                                   | **Yes**  |
| `-r` | The name of your Artifact Registry repository.                  | **Yes**  |
| `-i` | The name for your image.                                        | **Yes**  |
| `-l` | The region of your Artifact Registry. Defaults to `us-central1` | No       |
| `-h` | Display the help message.                                       | No       |

Sample command:

```bash
bash build-and-push-cos-nvidia-bug-report.sh \
    -p ${PROJECT?} \
    -r ${ARTIFACT_REPO?} \
    -i "custom-bug-report-collector" \
    -l "us-east1"
```
