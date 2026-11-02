# Copyright 2025 "Google LLC"
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Constants used in the bug report generation script."""

import immutabledict

METADATA_HEADERS = immutabledict.immutabledict({"Metadata-Flavor": "Google"})
METADATA_PREFIX = "http://metadata.google.internal/computeMetadata/v1/"

NVIDIA_BUG_REPORT_SCRIPT = "nvidia-bug-report.sh"
NVIDIA_BUG_REPORT_FLAGS = "--extra-system-data"
NVIDIA_BUG_REPORT_OUTPUT_NAME = "nvidia-bug-report.log.gz"
NVIDIA_SMI_BINARY_NAME = "nvidia-smi"
DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH = "/app/nvidia-bug-report.sh"

# Host state is bind-mounted same-path, so these are read directly.
HOST_OS_RELEASE_PATH = "/etc/os-release"

# The host's /usr is the one directory that cannot be mounted same-path: it
# would shadow the container's own userland, including the Python the entrypoint
# runs and the /usr/bin that MFT installs into at runtime.
HOST_USR_MOUNT_DIR = "/host_usr"

IMEX_CTL_BINARY_NAME = "nvidia-imex-ctl"
# RHEL/CentOS/Ubuntu package layout first, then the COS and GKE driver bundles.
IMEX_CONFIG_PATHS = (
    "/etc/nvidia-imex/config.cfg",
    "/var/lib/nvidia/imex-config/config.cfg",
    "/home/kubernetes/bin/nvidia/imex-config/config.cfg",
)
IMEX_LOG_PATH = "/var/log/nvidia-imex.log"

# Host directories that may hold the NVIDIA driver libraries: the COS and GKE
# driver bundles, then the package-managed layouts. The same directories are on
# LD_LIBRARY_PATH, see the Dockerfile.
NVIDIA_HOST_LIBRARY_DIRS = (
    "/var/lib/nvidia/lib64",
    "/home/kubernetes/bin/nvidia/lib64",
    "/usr/lib64",
    "/usr/lib/aarch64-linux-gnu",
    "/usr/lib/x86_64-linux-gnu",
)
# Libraries that tools dlopen by their unversioned name. The .run installer
# creates these aliases, but some distro driver packages ship only the soname.
NVIDIA_UNVERSIONED_LIBRARY_ALIASES = ("libnvidia-ml.so", "libcuda.so")
# Last on LD_LIBRARY_PATH, so an alias is used only if the host has none.
CONTAINER_NVIDIA_ALIAS_DIR = "/nvidia_alias"

# How the host's NVIDIA driver userspace (nvidia-smi, libnvidia-ml, ...) reaches
# the container. run-goog-gpu-bug-report.sh sets this environment variable to
# "nct_runtime_cdi" (injected through CDI by the NVIDIA Container Toolkit
# runtime) or "host_mounts" (possibly followed by why the toolkit was not used).
DRIVER_USERSPACE_ENV = "GOOG_GPU_BUG_REPORT_DRIVER_USERSPACE"
DRIVER_USERSPACE_NCT_RUNTIME_CDI = "nct_runtime_cdi"
# Sonames whose ldconfig cache entries locate the toolkit-injected libraries.
NVIDIA_LIBRARY_SONAMES = ("libnvidia-ml.so.1", "libcuda.so.1")
# Tools a run through the toolkit must have. The toolkit injects nvidia-smi;
# run-goog-gpu-bug-report.sh mounts nvidia-bug-report.sh, which the toolkit
# never injects.
CDI_REQUIRED_TOOLS = ("nvidia-smi", "nvidia-bug-report.sh")
# Exit code that tells run-goog-gpu-bug-report.sh that the toolkit's container
# lacks CDI_REQUIRED_TOOLS, so it retries with host mounts.
EXIT_CDI_INCOMPLETE = 3

IOSTAT_FILE_NAME = "iostat.txt"
IMEX_STATUS_FILE_NAME = "imex_status.txt"
IMEX_LOG_FILE_NAME = "nvidia_imex.log"
IBV_DEVINFO_FILE_NAME = "ibv_devinfo.txt"
NVIDIA_SMI_TOPO_FILE_NAME = "nvidia_smi_topo.txt"
BUG_REPORT_ARCHIVE_PREFIX = "goog_gpu_bug_report"


# Pinned fallback, used when the host has no signed kernel module bundle to
# download. COS instead resolves the newest version in its tools bucket.
MFT_DEFAULT_VERSION = "4.33.0"
MFT_DEFAULT_BUILD_VERSION = "169"
MFT_DOWNLOAD_URL_PREFIX = "https://www.mellanox.com/downloads/MFT/"
MFT_FILENAME_PREFIX = "mft"
MFT_FILENAME_SUFFIX = "deb.tgz"
MELLANOX_VENDOR_ID = "15b3"
MFT_KERNEL_MODULES_NAME = "mft_kernel_modules"
MFT_KERNEL_MODULES_GCS_NAMING_PATTERN = "mft-kernel-modules-*.tgz"
MFT_KERNEL_MODULES_LOCAL_DIR_PATH = "/var/lib/mft_kernel_modules"

BUG_REPORT_BUCKET_NAME = "vm-instance-gpu-bug-reports"

MST_PCI_KERNEL_MODULE_NAME = "mst_pci"
MST_PCICONF_KERNEL_MODULE_NAME = "mst_pciconf"

COS_TOOL_LOCATION_KEY_DEFAULT = "ARTIFACTS_LOCATION_US="
COS_TOOL_LOCATION_KEY_ASIA = "ARTIFACTS_LOCATION_ASIA="
COS_TOOL_LOCATION_KEY_EUROPE = "ARTIFACTS_LOCATION_EU="
COS_RELEASE_INFO_FILE_PATH = "/etc/lsb-release"
