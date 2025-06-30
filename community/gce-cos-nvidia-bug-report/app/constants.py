"""Constants used in the bug report generation script."""

import immutabledict

METADATA_HEADERS = immutabledict.immutabledict({"Metadata-Flavor": "Google"})
METADATA_PREFIX = "http://metadata.google.internal/computeMetadata/v1/"

NVIDIA_BUG_REPORT_SCRIPT = "nvidia-bug-report.sh"
NVIDIA_BUG_REPORT_FLAGS = "--extra-system-data"
NVIDIA_BUG_REPORT_OUTPUT_NAME = "nvidia-bug-report.log.gz"
NVIDIA_SMI_BINARY_NAME = "nvidia-smi"
DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH = "/app/nvidia-bug-report.sh"

# Sample MFT filename for ARM: "mft-4.30.1-113-arm64-deb.tgz"
# Sample MFT filename for x86: "mft-4.30.1-113-x86_64-deb.tgz"
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
COS_RELEASE_INFO_FILE_PATH = "/etc_host/lsb-release"
