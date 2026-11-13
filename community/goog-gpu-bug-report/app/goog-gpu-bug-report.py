# Copyright 2026 "Google LLC"
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

r"""Generates a bug report for GCE GPU VMs and uploads it to GCS bucket.

This script is for generating a bug report for Nvidia with Mellanox firmware
tool (MFT) logs included for certain GCE GPU machine types, and eventually
uploads the bug report along with some instance metadata to a GCS bucket
from the customer project.
"""

from collections.abc import Sequence
import dataclasses
import datetime
import enum
import os
import re
import shutil
import signal
import subprocess
import tarfile

from absl import app
from absl import flags
from absl import logging
import constants
from google.api_core import exceptions as google_api_exceptions
from google.cloud import storage
from packaging import version
import requests
import retry

_GCS_BUCKET = flags.DEFINE_string(
    "gcs_bucket",
    None,
    "The GCS bucket to upload the bug report to.",
)
_MFT_KERNEL_MODULES = flags.DEFINE_enum(
    "mft_kernel_modules",
    "auto",
    ["auto", "required", "never"],
    "Controls the MFT kernel modules (mst_pci, mst_pciconf) on hosts that need"
    " MFT. 'auto' (default): try to load them (COS pre-signed modules or DKMS"
    " on non-COS hosts) and silently fall back to driverless MFT on failure."
    " 'required': fail the run if the modules cannot be loaded (use to test"
    " the driver path). 'never': unload and block the modules so the"
    " driverless path is exercised (use to test the driverless path).",
)

# modprobe.d drop-in (inside the container) that prevents `mst start` from
# re-loading the MFT kernel modules when --mft_kernel_modules=never.
_MODPROBE_BLOCK_FILE = "/etc/modprobe.d/mst-blacklist.conf"


class MftKernelModulesError(RuntimeError):
  """Raised when --mft_kernel_modules=required cannot be satisfied."""


@dataclasses.dataclass
class Artifact:
  """Dataclass to hold generated artifact."""

  filepath: str
  content_type: str = "application/octet-stream"


class Architecture(enum.Enum):
  X86 = "x86_64"
  ARM = "arm64"


@dataclasses.dataclass
class GceInstanceInfo:
  """Dataclass to hold GCE instance information."""

  project_id: str = None
  instance_id: str = None
  zone: str = None
  image: str = None
  machine_type: str = None
  architecture: str = None
  mst_version: str = None
  mft_kernel_modules_loaded: bool = None
  driver_userspace: str = None

  def __str__(self) -> str:
    return (
        "GCE Instance Info:\n"
        f"\tProject ID: {self.project_id}\n"
        f"\tInstance ID: {self.instance_id}\n"
        f"\tImage: {self.image}\n"
        f"\tZone: {self.zone}\n"
        f"\tMachine Type: {self.machine_type}\n"
        f"\tArchitecture: {self.architecture}\n"
        f"\tMST Version: {self.mst_version}\n"
        f"\tMFT Kernel Modules Loaded: {self.mft_kernel_modules_loaded}\n"
        f"\tNVIDIA Driver Userspace: {self.driver_userspace}\n"
    )


def RunCommand(
    command: str, timeout_sec: int = 90, check_retcode: bool = True
) -> tuple[int, str, str]:
  """Executes the given shell command.

  Args:
    command: the shell command to run on the VM.
    timeout_sec: the timeout period to run the command.
    check_retcode: If true, checks the return code of the command.

  Returns:
    The return code of the process, standard output, and standard error of
    executing the command.

  Raises:
    subprocess.TimeoutExpired: If the command did not finish within the timeout
      period, a subprocess.TimeoutExpired error would get raised.
    subprocess.CalledProcessError: If the command returns a non-zero
      exit code.
  """
  process = subprocess.Popen(
      command,
      shell=True,
      stdout=subprocess.PIPE,
      stderr=subprocess.PIPE,
      start_new_session=True,
  )
  logging.debug("Running command: %s", command)
  try:
    stdout, stderr = process.communicate(timeout=timeout_sec)
  except subprocess.TimeoutExpired:
    # Kill the whole process group so shell children do not outlive timeout.
    try:
      os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
      pass
    process.communicate()
    raise
  stdout = stdout.decode("utf-8", errors="replace")
  stderr = stderr.decode("utf-8", errors="replace")
  logging.debug(
      "\n===\ncommand:\n%s\nstdout:\n%s\nstderr:\n%s\nreturn code: %s\n===",
      command,
      stdout,
      stderr,
      process.returncode,
  )
  if check_retcode and process.returncode != 0:
    raise subprocess.CalledProcessError(
        process.returncode, command, stdout, stderr
    )
  return process.returncode, stdout, stderr


def GetHostArchitecture() -> Architecture:
  """Returns the host CPU architecture from Linux uname(2)."""
  machine = os.uname().machine
  if machine == "aarch64":
    return Architecture.ARM
  if machine == "x86_64":
    return Architecture.X86
  raise ValueError(f"Unsupported host architecture: {machine}")


@retry.retry(
    requests.exceptions.RequestException,
    tries=5,
    delay=1,
    backoff=1.5,
    max_delay=3,
)
def QueryMetadataServerOrDie(metadata_key: str) -> str:
  """Queries the metadata server for the given path.

  Args:
    metadata_key: The metadata key to query.

  Returns:
    The value of the metadata key.

  Raises:
    requests.exceptions.RequestException: If the request fails consistently
    after retries.
  """
  metadata_url = f"{constants.METADATA_PREFIX}{metadata_key}"
  response = requests.get(
      metadata_url, headers=constants.METADATA_HEADERS, timeout=5
  )
  response.raise_for_status()
  return response.text


def GetGceInstanceInformationOrDie() -> GceInstanceInfo:
  """Queries the metadata server for GCE instance information.

  Returns:
    A GceInstanceInfo object containing the GCE instance information. This
    includes the project ID, instance ID, zone, image, machine type, and
    architecture.
  """
  project_id: str = QueryMetadataServerOrDie("project/project-id")
  instance_id: str = QueryMetadataServerOrDie("instance/id")
  zone: str = QueryMetadataServerOrDie("instance/zone").split("/")[-1]
  image: str = QueryMetadataServerOrDie("instance/image").split("/")[-1]
  machine_type: str = (
      QueryMetadataServerOrDie("instance/machine-type").split("/")[-1].lower()
  )

  if "gpu" not in machine_type:
    raise ValueError(f"{machine_type} is not a GPU machine type.")

  architecture = GetHostArchitecture()

  gce_instance_info = GceInstanceInfo(
      project_id=project_id,
      instance_id=instance_id,
      zone=zone,
      image=image,
      machine_type=machine_type,
      architecture=architecture,
  )

  return gce_instance_info


def UploadArtifactToGcs(
    artifact: Artifact,
    bucket: storage.Bucket,
    destination_directory: str,
) -> None:
  """Uploads a file to GCS.

  Args:
    artifact: The artifact to upload.
    bucket: The GCS bucket name to upload the artifact to.
    destination_directory: The destination directory on GCS.
  """
  destination_path = os.path.join(
      destination_directory, os.path.basename(artifact.filepath)
  )
  logging.debug(
      "Uploading %s to GCS: %s/%s",
      artifact.filepath,
      bucket.name,
      destination_path,
  )
  blob = bucket.blob(destination_path)
  blob.upload_from_filename(
      artifact.filepath, content_type=artifact.content_type
  )


def TransferArtifacts(
    upload: bool,
    artifact: Artifact,
    bucket: storage.Bucket,
    gcs_path: str,
) -> None:
  """Uploads or downloads an artifact to/from a GCS path.

  Args:
    upload: The transfer direction (True for upload, False for download).
    artifact: The artifact to be uploaded or downloaded.
    bucket: The GCS bucket name to upload to or download from.
    gcs_path: The directory of the object within the GCS bucket.
  """
  blob = bucket.blob(gcs_path)

  if upload:
    logging.debug(
        "Uploading %s to GCS %s/%s",
        artifact.filepath,
        bucket.name,
        gcs_path,
    )
    blob.upload_from_filename(
        artifact.filepath, content_type=artifact.content_type
    )
  else:
    logging.debug(
        "Downloading from GCS %s/%s to %s",
        bucket.name,
        gcs_path,
        artifact.filepath,
    )
    os.makedirs(os.path.dirname(artifact.filepath), exist_ok=True)
    blob.download_to_filename(artifact.filepath)


def ListObjectsInGcs(
    bucket: storage.Bucket | str, glob_pattern: str
) -> Sequence[storage.Blob]:
  """Lists objects in a GCS bucket.

  Args:
    bucket: The GCS bucket name or a storage.Bucket object.
    glob_pattern: The glob pattern to match the objects.

  Returns:
    A sequence of storage.Blob objects that match the glob pattern.
  """
  if isinstance(bucket, str):
    storage_client = storage.Client()
    bucket = storage_client.bucket(bucket)
  logging.debug(
      "Listing objects in GCS bucket %s with pattern: %s",
      bucket.name,
      glob_pattern,
  )
  return list(bucket.list_blobs(match_glob=glob_pattern))


def CreateMasterBugReportArchive(
    artifacts: Sequence[Artifact],
    output_directory: str,
    instance_id: str,
    timestamp: str,
) -> Artifact:
  """Bundles all diagnostic artifacts into a single compressed master tarball.

  Args:
    artifacts: List of Artifact objects generated during bug report creation.
    output_directory: Local directory where artifacts are stored.
    instance_id: GCE VM Instance ID.
    timestamp: UTC timestamp string.

  Returns:
    An Artifact pointing to the created master .tar.gz bundle.
  """
  archive_filename = (
      f"{constants.BUG_REPORT_ARCHIVE_PREFIX}_{instance_id}_{timestamp}.tar.gz"
  )
  archive_filepath = os.path.join(output_directory, archive_filename)

  logging.info("Creating master bug report archive at: %s", archive_filepath)
  with tarfile.open(archive_filepath, "w:gz") as tar:
    for artifact in artifacts:
      if os.path.exists(artifact.filepath):
        arcname = os.path.basename(artifact.filepath)
        tar.add(artifact.filepath, arcname=arcname)
      else:
        logging.warning(
            "Artifact file missing when archiving: %s", artifact.filepath
        )

  return Artifact(filepath=archive_filepath, content_type="application/gzip")


def UploadArtifactsToGcs(
    artifacts_to_upload: Sequence[Artifact],
    bucket_name: str,
    destination_directory: str,
) -> None:
  """Uploads a file to GCS.

  Args:
    artifacts_to_upload: A sequence of artifact to be uploaded to GCS.
    bucket_name: Destination GCS bucket name.
    destination_directory: The destination directory on GCS.

  Raises:
    ValueError: If no files are provided or if any of the files do not exist.
  """
  if not artifacts_to_upload:
    raise ValueError("No files to upload.")

  for artifact in artifacts_to_upload:
    if not os.path.exists(artifact.filepath):
      raise ValueError(f"File {artifact.filepath} does not exist.")

  storage_client = storage.Client()

  try:
    bucket = storage_client.lookup_bucket(bucket_name)
  except google_api_exceptions.GoogleAPIError as e:
    logging.warning(
        "Bucket lookup failed for %s: %s. Skipping GCS upload.",
        bucket_name,
        e,
    )
    return

  if bucket is None:
    # Create the bucket if it does not exist.
    try:
      bucket = storage_client.create_bucket(bucket_name)
      logging.info("Created bucket [%s] in the project.", bucket_name)
    except google_api_exceptions.GoogleAPIError as e:
      logging.warning(
          "Bucket creation failed for %s: %s. Skipping GCS upload.",
          bucket_name,
          e,
      )
      return

  for artifact in artifacts_to_upload:
    UploadArtifactToGcs(
        artifact=artifact,
        bucket=bucket,
        destination_directory=destination_directory,
    )


def CollectIostat(output_directory: str) -> Artifact:
  """Collects storage and I/O statistics using iostat."""
  filepath = os.path.join(output_directory, constants.IOSTAT_FILE_NAME)
  iostat_path = shutil.which("iostat")
  if not iostat_path:
    logging.warning("iostat command not found on system.")
    with open(filepath, "w") as file_handle:
      file_handle.write("iostat command not available on this host system.\n")
    return Artifact(filepath=filepath, content_type="text/plain")

  ret_code, stdout, stderr = RunCommand(
      command="iostat -xt", timeout_sec=30, check_retcode=False
  )
  with open(filepath, "w") as file_handle:
    if ret_code == 0 and stdout:
      file_handle.write(stdout)
    else:
      file_handle.write(
          f"Failed to run iostat (exit code {ret_code}):\n{stderr}\n"
      )

  return Artifact(filepath=filepath, content_type="text/plain")


def IsMnnvlSupportedMachineType(machine_type: str) -> bool:
  """Returns True if the machine type supports multi-node NVLink (MNNVL/NVLD)."""
  if not machine_type:
    return False
  match = re.match(r"^a(\d+)x", machine_type.lower())
  return match is not None and int(match.group(1)) >= 4


def IsConnectxNicSupportedMachineType(machine_type: str) -> bool:
  """Returns True if the machine type uses NVIDIA ConnectX NICs."""
  if not machine_type:
    return False
  if machine_type.lower().startswith("a3-ultragpu"):
    return True
  match = re.match(r"^a(\d+)", machine_type.lower())
  return match is not None and int(match.group(1)) >= 4


def CollectImexLogs(
    output_directory: str, gce_instance_info: GceInstanceInfo
) -> Sequence[Artifact]:
  """Collects NVIDIA IMEX daemon status and logs for multi-node NVLink domains."""
  artifacts = []
  status_filepath = os.path.join(
      output_directory, constants.IMEX_STATUS_FILE_NAME
  )
  log_filepath = os.path.join(output_directory, constants.IMEX_LOG_FILE_NAME)

  if not IsMnnvlSupportedMachineType(gce_instance_info.machine_type):
    logging.info(
        "IMEX is not applicable for machine type: %s",
        gce_instance_info.machine_type,
    )
    with open(status_filepath, "w") as file_handle:
      file_handle.write(
          "IMEX is only applicable to NVLink Domain (NVLD) based products"
          " (e.g., A4X, A4X Max, A5X). Current machine type:"
          f" {gce_instance_info.machine_type}.\n"
      )
    with open(log_filepath, "w") as file_handle:
      file_handle.write(
          "IMEX is only applicable to NVLink Domain (NVLD) based products"
          " (e.g., A4X, A4X Max, A5X). Current machine type:"
          f" {gce_instance_info.machine_type}.\n"
      )
    artifacts.append(
        Artifact(filepath=status_filepath, content_type="text/plain")
    )
    artifacts.append(Artifact(filepath=log_filepath, content_type="text/plain"))
    return artifacts

  # 1. IMEX Status via nvidia-imex-ctl
  imex_ctl_path = shutil.which(constants.IMEX_CTL_BINARY_NAME)

  # nvidia-imex-ctl defaults to /etc/nvidia-imex/config.cfg, which only exists
  # in the package-managed layout; COS and GKE need -c pointed at the bundle.
  imex_config_path = None
  for candidate_path in constants.IMEX_CONFIG_PATHS:
    container_path = HostPath(candidate_path)
    if os.path.exists(container_path):
      imex_config_path = container_path
      break

  if imex_ctl_path:
    config_opt = f"-c {imex_config_path} " if imex_config_path else ""
    ret_code, stdout, stderr = RunCommand(
        command=f"{imex_ctl_path} {config_opt}-N",
        timeout_sec=30,
        check_retcode=False,
    )
    with open(status_filepath, "w") as file_handle:
      if ret_code == 0 and stdout:
        file_handle.write(stdout)
      else:
        file_handle.write(
            f"IMEX status command failed (exit code {ret_code}):\n{stderr}\n"
        )
  else:
    with open(status_filepath, "w") as file_handle:
      file_handle.write("nvidia-imex-ctl binary not present on this host VM.\n")

  artifacts.append(
      Artifact(filepath=status_filepath, content_type="text/plain")
  )

  # 2. IMEX Log File from the host's /var/log.
  imex_host_log = HostPath(constants.IMEX_LOG_PATH)
  if os.path.exists(imex_host_log):
    try:
      shutil.copy(imex_host_log, log_filepath)
    except OSError as e:
      logging.warning("Failed to copy IMEX log file: %s", e)
      with open(log_filepath, "w") as file_handle:
        file_handle.write(f"Error copying IMEX log file {imex_host_log}: {e}\n")
  else:
    with open(log_filepath, "w") as file_handle:
      file_handle.write("IMEX log file not found on host system.\n")

  artifacts.append(Artifact(filepath=log_filepath, content_type="text/plain"))
  return artifacts


def CollectIbvDevinfo(
    output_directory: str, gce_instance_info: GceInstanceInfo
) -> Artifact:
  """Collects RDMA Verbs HCA capabilities and port link states using ibv_devinfo."""
  filepath = os.path.join(output_directory, constants.IBV_DEVINFO_FILE_NAME)
  if not IsConnectxNicSupportedMachineType(gce_instance_info.machine_type):
    with open(filepath, "w") as file_handle:
      file_handle.write(
          "RDMA Verbs HCA capabilities are only applicable to machine types"
          " with ConnectX NICs (e.g., A3 Ultra, A4, A4X). Current machine"
          f" type: {gce_instance_info.machine_type}.\n"
      )
    return Artifact(filepath=filepath, content_type="text/plain")

  ibv_path = shutil.which("ibv_devinfo")
  if not ibv_path:
    logging.warning("ibv_devinfo command not found on system.")
    with open(filepath, "w") as file_handle:
      file_handle.write(
          "ibv_devinfo command not available on this host system.\n"
      )
    return Artifact(filepath=filepath, content_type="text/plain")

  ret_code, stdout, stderr = RunCommand(
      command="ibv_devinfo -v", timeout_sec=30, check_retcode=False
  )
  with open(filepath, "w") as file_handle:
    if ret_code == 0 and stdout:
      file_handle.write(stdout)
    else:
      file_handle.write(
          "ibv_devinfo failed or no RDMA devices found (exit code"
          f" {ret_code}):\n{stderr}\n"
      )

  return Artifact(filepath=filepath, content_type="text/plain")


def CollectNvidiaSmiTopo(output_directory: str) -> Artifact:
  """Collects GPU and NIC hardware topology matrix using nvidia-smi topo -m."""
  filepath = os.path.join(output_directory, constants.NVIDIA_SMI_TOPO_FILE_NAME)
  smi_path = shutil.which(constants.NVIDIA_SMI_BINARY_NAME)
  if not smi_path:
    logging.warning("nvidia-smi binary not found on system.")
    with open(filepath, "w") as file_handle:
      file_handle.write(
          "nvidia-smi binary not available on this host system.\n"
      )
    return Artifact(filepath=filepath, content_type="text/plain")

  ret_code, stdout, stderr = RunCommand(
      command=f"{constants.NVIDIA_SMI_BINARY_NAME} topo -m",
      timeout_sec=30,
      check_retcode=False,
  )
  with open(filepath, "w") as file_handle:
    if ret_code == 0 and stdout:
      file_handle.write(stdout)
    else:
      file_handle.write(
          "nvidia-smi topo -m failed or is not supported (exit code"
          f" {ret_code}):\n{stderr}\n"
      )

  return Artifact(filepath=filepath, content_type="text/plain")


def GenerateBugReport(
    gce_instance_info: GceInstanceInfo,
    output_directory: str = "/tmp/goog_gpu_bug_reports",
) -> Sequence[Artifact]:
  """Generates NVIDIA bug report and additional hardware diagnostic log artifacts.

  Executes the host nvidia-bug-report.sh script (if present on the system) and
  collects diagnostic logs (iostat, IMEX, ibv_devinfo, nvidia-smi topo)
  without crashing if GPU drivers or scripts are absent.

  Args:
    gce_instance_info: Information about the running GCE VM instance.
    output_directory: Directory path where generated artifact files are stored.

  Returns:
    A sequence of Artifact objects representing generated log files.
  """
  final_nvidia_bug_report_output_path = None
  default_nvidia_bug_report_output_path = os.path.join(
      os.getcwd(), constants.NVIDIA_BUG_REPORT_OUTPUT_NAME
  )
  # Remove the current bug report if it already exists
  if os.path.exists(default_nvidia_bug_report_output_path):
    logging.info(
        "Removing an existing NVIDIA bug report at: %s",
        default_nvidia_bug_report_output_path,
    )
    os.remove(default_nvidia_bug_report_output_path)
  os.makedirs(output_directory, exist_ok=True)
  nvidia_bug_report_script_path = shutil.which(
      constants.NVIDIA_BUG_REPORT_SCRIPT
  )
  if nvidia_bug_report_script_path:
    shutil.copy(
        nvidia_bug_report_script_path,
        constants.DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH,
    )
    # Avoid reporting the return code of the mst stop command. This would cause
    # the script to hit some non-trivial bugs from MST where the kernel module
    # is in fact loaded yet it was complaining not able to locate it.
    # Sample error message:
    # Stopping MST (Mellanox Software Tools) driver set
    # Unloading MST PCI configuration modulemodprobe: FATAL: Module mst_pciconf
    # not found.
    #  - Failure: 1
    # Unloading MST PCI modulemodprobe: FATAL: Module mst_pci not found.
    #  - Failure: 1
    RunCommand(
        command=(
            "sed -i 's/report_command \"\\$mst stop\"/\\$mst stop/g'"
            f" {constants.DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH}"
        )
    )
    # Removing the sudo from the duplicated script because sudo is not supported
    # inside the docker container.
    RunCommand(
        command=(
            "sed -i 's/sudo //g'"
            f" {constants.DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH}"
        )
    )
    # Ensure the duplicated script is executable.
    RunCommand(
        command=(
            f"chmod 777 {constants.DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH}"
        )
    )
    # Generate the NVIDIA bug report.
    try:
      ret_code, _, stderr = RunCommand(
          command=(
              f"{constants.DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH}"
              f" {constants.NVIDIA_BUG_REPORT_FLAGS}"
          ),
          timeout_sec=600,
          check_retcode=False,
      )
      if ret_code != 0:
        logging.warning(
            "%s exited with code %d: %s",
            constants.NVIDIA_BUG_REPORT_SCRIPT,
            ret_code,
            stderr,
        )
    except subprocess.TimeoutExpired:
      logging.warning(
          "%s timed out after 600s; continuing with remaining diagnostics.",
          constants.NVIDIA_BUG_REPORT_SCRIPT,
      )
    if os.path.exists(default_nvidia_bug_report_output_path):
      final_nvidia_bug_report_output_path = os.path.join(
          output_directory,
          constants.NVIDIA_BUG_REPORT_OUTPUT_NAME,
      )
      shutil.move(
          default_nvidia_bug_report_output_path,
          final_nvidia_bug_report_output_path,
      )
  else:
    logging.warning(
        "%s script not found. Skipping NVIDIA bug report generation.",
        constants.NVIDIA_BUG_REPORT_SCRIPT,
    )

  final_instance_info_path = os.path.join(
      output_directory,
      "instance_info.txt",
  )
  with open(final_instance_info_path, "w") as file_handle:
    file_handle.write(str(gce_instance_info))

  artifacts = []
  if final_nvidia_bug_report_output_path:
    artifacts.append(
        Artifact(
            filepath=final_nvidia_bug_report_output_path,
            content_type="application/gzip",
        )
    )
  artifacts.append(
      Artifact(
          filepath=final_instance_info_path,
          content_type="text/plain",
      )
  )

  # Collect 4 additional diagnostic logs
  artifacts.append(CollectIostat(output_directory))
  artifacts.extend(CollectImexLogs(output_directory, gce_instance_info))
  artifacts.append(CollectIbvDevinfo(output_directory, gce_instance_info))
  artifacts.append(CollectNvidiaSmiTopo(output_directory))

  for artifact in artifacts:
    logging.debug(
        "Generated bug report file: %s",
        artifact.filepath,
    )

  return artifacts


@retry.retry(
    requests.exceptions.RequestException,
    tries=5,
    delay=1,
    backoff=1.5,
    max_delay=3,
)
def DownloadFileFromUrl(url: str, destination_path: str) -> None:
  """Downloads a file from a URL and saves it to the current working directory.

  Args:
    url: The URL to download the file from.
    destination_path: The path to save the file to.

  Raises:
    requests.exceptions.RequestException: If the download request fails
    consistently after retries.
  """
  if os.path.exists(destination_path):
    logging.info("Removing already existing file at: %s", destination_path)
    os.remove(destination_path)

  response = requests.get(url, stream=True, timeout=60)
  response.raise_for_status()
  with open(destination_path, "wb") as file_handle:
    for chunk in response.iter_content(chunk_size=8192):
      file_handle.write(chunk)
  logging.info("Downloaded file to: %s", destination_path)


def DownloadAndUnpackMft(
    architecture: Architecture, mft_version: str, mft_build_version: str
) -> str:
  """Downloads and unpacks MFT software, returning the installer path.

  Args:
    architecture: The architecture of the GCE instance. This can be X86 or ARM.
    mft_version: The version of the MFT to install.
    mft_build_version: The build version of the MFT to install.

  Returns:
    The absolute file path to install.sh.

  Raises:
    ValueError: If architecture is unsupported or unpacked installer is missing.
  """
  if architecture != Architecture.X86 and architecture != Architecture.ARM:
    raise ValueError(f"Unsupported architecture: {architecture}")
  logging.info(
      "Preparing MFT software for architecture: %s",
      architecture.value,
  )
  mft_filename = "-".join([
      constants.MFT_FILENAME_PREFIX,
      mft_version,
      mft_build_version,
      architecture.value,
      constants.MFT_FILENAME_SUFFIX,
  ])

  mft_download_url = constants.MFT_DOWNLOAD_URL_PREFIX + mft_filename
  mft_filename_path = os.path.join("/tmp", mft_filename)

  if os.path.exists(mft_filename_path):
    logging.debug("MFT is already present locally at: %s", mft_filename_path)
  else:
    logging.info(
        "Downloading MFT from: %s to %s",
        mft_download_url,
        mft_filename_path,
    )
    DownloadFileFromUrl(mft_download_url, mft_filename_path)
  logging.info("Unpacking MFT: %s", mft_filename_path)
  unpacked_mft_directory_path = mft_filename_path.removesuffix(".tgz")
  if not os.path.exists(unpacked_mft_directory_path):
    RunCommand(command=f"tar -xzvf {mft_filename_path} -C /tmp")
  if not os.path.exists(unpacked_mft_directory_path):
    raise ValueError(
        f"MFT unpacked directory does not exist: {unpacked_mft_directory_path}"
    )
  mft_installer_path = f"{unpacked_mft_directory_path}/install.sh"
  if not os.path.exists(mft_installer_path):
    raise ValueError(
        f"MFT installer does not exist. Expected at: {mft_installer_path}"
    )
  return mft_installer_path


def InstallMftUserspaceSoftware(mft_installer_path: str) -> None:
  """Installs Mellanox Firmware Tool (MFT) userspace software inside container.

  Args:
    mft_installer_path: File path to the unpacked MFT install.sh script.
  """
  logging.debug("Installing MFT through installer: %s", mft_installer_path)
  RunCommand(
      command=f"bash {mft_installer_path} --without-kernel",
      timeout_sec=600,
  )
  logging.info("MFT installation completed.")


def InstallMftKernelModulesViaDkms(mft_installer_path: str) -> bool:
  """Attempts DKMS compilation of MFT kernel drivers on Linux hosts.

  Args:
    mft_installer_path: File path to the unpacked MFT install.sh script.

  Returns:
    True if kernel drivers were compiled and loaded, False otherwise.
  """
  if IsKernelModuleLoaded(
      constants.MST_PCI_KERNEL_MODULE_NAME
  ) and IsKernelModuleLoaded(constants.MST_PCICONF_KERNEL_MODULE_NAME):
    logging.info("MFT kernel drivers are already loaded on host.")
    return True

  kernel_version = os.uname().release
  logging.info(
      "Attempting MFT kernel driver compilation for host kernel: %s",
      kernel_version,
  )

  # Check if host kernel headers exist.
  build_path = f"/lib/modules/{kernel_version}/build"
  if not (
      os.path.exists(build_path) and os.path.isdir(os.path.realpath(build_path))
  ):
    logging.warning(
        "Host kernel headers missing at %s. Skipping DKMS build.",
        build_path,
    )
    return False

  try:
    # Run installer to compile kernel module.
    RunCommand(
        command=f"bash {mft_installer_path}",
        timeout_sec=900,
    )
    for module in (
        constants.MST_PCI_KERNEL_MODULE_NAME,
        constants.MST_PCICONF_KERNEL_MODULE_NAME,
    ):
      RunCommand(command=f"modprobe {module}", timeout_sec=60)
    logging.info("Successfully compiled and loaded MFT drivers.")
    return True
  except Exception as e:  # pylint: disable=broad-exception-caught
    logging.warning(
        "DKMS driver compilation failed: %s. Falling back to driverless mode.",
        e,
    )
    return False


def HostPath(host_path: str) -> str:
  """Maps a path on the host to where it is visible inside the container.

  Host state is bind-mounted same-path, so this is the identity for everything
  but /usr. Use it for reading host state only; paths the container writes to
  are always same-path.

  Args:
    host_path: absolute path as it exists on the host.

  Returns:
    The corresponding path in the container's mount namespace.
  """
  if host_path == "/usr" or host_path.startswith("/usr/"):
    return constants.HOST_USR_MOUNT_DIR + host_path.removeprefix("/usr")
  return host_path


def IsCosHost() -> bool:
  """Checks if the host OS is Google Container-Optimized OS (COS).

  Returns:
    True if the host's os-release indicates COS, False otherwise or if the
    host's os-release is not mounted.
  """
  os_release_path = HostPath(constants.HOST_OS_RELEASE_PATH)
  if not os.path.exists(os_release_path):
    logging.warning("Host OS release file %s does not exist.", os_release_path)
    return False
  try:
    with open(os_release_path, "r") as file_handle:
      content = file_handle.read()
      is_cos = "ID=cos" in content or 'NAME="Container-Optimized OS"' in content
      # Logged because a missing bind mount silently yields the container's own
      # os-release, which would route a COS host down the DKMS branch.
      logging.info("Host OS read from %s: COS=%s", os_release_path, is_cos)
      return is_cos
  except OSError as e:
    logging.warning("Failed to read %s: %s", os_release_path, e)
    return False


def _LdconfigLibraryDirs(sonames: Sequence[str]) -> list[str]:
  """Returns the directories the ldconfig cache lists for the given sonames."""
  ret_code, stdout, _ = RunCommand(
      command="ldconfig -p", timeout_sec=30, check_retcode=False
  )
  if ret_code != 0:
    return []
  library_dirs = []
  for line in stdout.splitlines():
    # e.g. "libnvidia-ml.so.1 (libc6,x86-64) => /usr/lib64/libnvidia-ml.so.1"
    entry, separator, path = line.strip().partition(" => ")
    if separator and entry.split(" ", 1)[0] in sonames:
      library_dirs.append(os.path.dirname(path))
  return library_dirs


def NvidiaLibraryDirs() -> list[str]:
  """Returns the container directories holding the host's NVIDIA libraries.

  These are the bind-mounted host library directories (host_mounts), followed
  by the directories where the container's ldconfig cache finds the NVIDIA
  sonames (nct_runtime_cdi). The NVIDIA Container Toolkit mounts the libraries
  at their host paths relative to the driver root, e.g. /usr/lib64 on Rocky and
  /lib64 on COS, and adds those directories to the cache. Missing directories
  are dropped.
  """
  candidates = [
      HostPath(host_dir) for host_dir in constants.NVIDIA_HOST_LIBRARY_DIRS
  ]
  candidates.extend(_LdconfigLibraryDirs(constants.NVIDIA_LIBRARY_SONAMES))
  # dict.fromkeys drops duplicates and keeps the order, which decides the
  # directory that LinkUnversionedLibraryAliases() links to.
  return [d for d in dict.fromkeys(candidates) if os.path.isdir(d)]


def LinkUnversionedLibraryAliases() -> None:
  """Adds the unversioned lib*.so names that some tools dlopen directly.

  `mst gpu add` dlopens "libnvidia-ml.so" verbatim, and a host's driver package
  may ship only the soname and the real file. The NVIDIA Container Toolkit does
  not create the unversioned name either unless the host has it. An alias is
  created only if no NVIDIA library directory already has the unversioned name.
  """
  library_dirs = NvidiaLibraryDirs()
  for alias_name in constants.NVIDIA_UNVERSIONED_LIBRARY_ALIASES:
    if any(
        os.path.exists(os.path.join(library_dir, alias_name))
        for library_dir in library_dirs
    ):
      continue
    target_path = None
    for library_dir in library_dirs:
      # Sorted, so an alias prefers the soname over the fully versioned file.
      versioned_names = sorted(
          name
          for name in os.listdir(library_dir)
          if name.startswith(f"{alias_name}.")
      )
      if versioned_names:
        target_path = os.path.join(library_dir, versioned_names[0])
        break
    if target_path is None:
      continue
    alias_path = os.path.join(constants.CONTAINER_NVIDIA_ALIAS_DIR, alias_name)
    try:
      os.makedirs(constants.CONTAINER_NVIDIA_ALIAS_DIR, exist_ok=True)
      if os.path.lexists(alias_path):
        os.unlink(alias_path)
      os.symlink(target_path, alias_path)
      logging.info("Linked %s to %s.", alias_path, target_path)
    except OSError as e:
      logging.warning("Unable to link %s: %s", alias_path, e)


def IsNvidiaDriverInstalled() -> bool:
  """Checks if the NVIDIA driver is installed.

  This function checks if the NVIDIA driver is installed by checking if the
  NVIDIA SMI binary is properly mounted to the specified path.

  Returns:
    True if the NVIDIA driver is installed, False otherwise.
  """
  nvidia_smi_binary_path = shutil.which(constants.NVIDIA_SMI_BINARY_NAME)
  return nvidia_smi_binary_path is not None


def ExtractCosToolsBucketInfo(
    gce_instance_info: GceInstanceInfo,
) -> tuple[str, str]:
  """Extracts the COS tools bucket path from the instance.

  The function checks the COS release info file bind-mounted at
  /etc/lsb-release and extracts the closest COS tools that matches the COS
  release information and the physical zone of the instance.

  Args:
    gce_instance_info: The GCE instance information.

  Returns:
    A tuple of (bucket_name, bucket_path) where bucket_name is the name of the
    COS tools bucket and bucket_path is the path of the bucket within the
    artifacts location.

  Raises:
    FileNotFoundError: If the COS release info file does not exist.
    ValueError: If the COS tools bucket path is not found in the COS release
    info file or if the bucket path is not a valid GCS path.
  """
  artifacts_location_key = constants.COS_TOOL_LOCATION_KEY_DEFAULT
  if gce_instance_info.zone.startswith("asia-"):
    artifacts_location_key = constants.COS_TOOL_LOCATION_KEY_ASIA
  elif gce_instance_info.zone.startswith("europe-"):
    artifacts_location_key = constants.COS_TOOL_LOCATION_KEY_EUROPE

  if not os.path.exists(constants.COS_RELEASE_INFO_FILE_PATH):
    raise FileNotFoundError(
        "COS release info file does not exist:"
        f" {constants.COS_RELEASE_INFO_FILE_PATH}"
    )
  cos_tools_path = None
  with open(constants.COS_RELEASE_INFO_FILE_PATH, "r") as f:
    for line in f:
      if line.startswith(artifacts_location_key):
        cos_tools_path = line.split("=")[1].rstrip()
  if cos_tools_path is None:
    raise ValueError(
        f"Did not found the COS tools bucket path {artifacts_location_key} in"
        f" the COS release info file {constants.COS_RELEASE_INFO_FILE_PATH}."
    )
  if not cos_tools_path.startswith("gs://"):
    raise ValueError(
        "Expecting COS tools bucket path to start with `gs://`, but got:"
        f" {cos_tools_path}"
    )
  cos_path_components = cos_tools_path.replace("gs://", "").split("/")
  bucket_name = cos_path_components[0]
  bucket_path = "/".join(cos_path_components[1:])
  return bucket_name, bucket_path


def IsMftNecessaryForMachineType(machine_type: str) -> bool:
  """Returns True if the machine type needs MFT data (A4 and later series)."""
  if not machine_type:
    return False
  match = re.match(r"^a(\d+)", machine_type.lower())
  return match is not None and int(match.group(1)) >= 4


def GetMftVersion() -> str:
  """Returns the Mellanox Firmware Tool (MFT) version."""
  _, mft_version, _ = RunCommand(command="mst version")
  return mft_version.rstrip()


def IsMftInstalled() -> bool:
  """Checks if the Mellanox Firmware Tool (MFT) is installed."""
  ret_code, _, _ = RunCommand(command="mst version", check_retcode=False)
  return ret_code == 0


def IsKernelModuleLoaded(kernel_module_name: str) -> bool:
  """Checks if the kernel module is loaded."""
  ret_code, _, _ = RunCommand(
      command=f"lsmod | grep {kernel_module_name}", check_retcode=False
  )
  is_loaded = ret_code == 0
  logging.info(
      "Is kernel module (%s) loaded: %s", kernel_module_name, is_loaded
  )
  return is_loaded


def AreMftKernelModulesLoaded() -> bool:
  """Returns True if both MFT kernel modules (mst_pci, mst_pciconf) are loaded."""
  return IsKernelModuleLoaded(
      constants.MST_PCI_KERNEL_MODULE_NAME
  ) and IsKernelModuleLoaded(constants.MST_PCICONF_KERNEL_MODULE_NAME)


def DisableMftKernelModules() -> None:
  """Unloads the MFT kernel modules and blocks them from being re-loaded.

  Used by --mft_kernel_modules=never to force the driverless MFT path. Besides
  rmmod, an `install <module> /bin/false` modprobe.d drop-in is written inside
  the container so that `mst start` (invoked by nvidia-bug-report.sh) cannot
  modprobe the modules back in from a stale DKMS build left in the host's
  /lib/modules. RemoveMftKernelModulesBlock() deletes the drop-in again.
  """
  # mst_pciconf depends on mst_pci, so unload it first.
  for module in (
      constants.MST_PCICONF_KERNEL_MODULE_NAME,
      constants.MST_PCI_KERNEL_MODULE_NAME,
  ):
    RunCommand(command=f"rmmod {module}", check_retcode=False)
  RunCommand(
      command=(
          f'echo "install {constants.MST_PCI_KERNEL_MODULE_NAME} /bin/false"'
          f" > {_MODPROBE_BLOCK_FILE}"
      )
  )
  RunCommand(
      command=(
          f'echo "install {constants.MST_PCICONF_KERNEL_MODULE_NAME}'
          f' /bin/false" >> {_MODPROBE_BLOCK_FILE}'
      )
  )
  logging.info(
      "MFT kernel modules disabled (--mft_kernel_modules=never); modprobe"
      " blocked via %s",
      _MODPROBE_BLOCK_FILE,
  )


def RemoveMftKernelModulesBlock() -> None:
  """Deletes the modprobe.d drop-in written by DisableMftKernelModules().

  Called before MFT setup (so a stale drop-in from an earlier run in a reused
  container cannot break --mft_kernel_modules=auto/required) and again at the
  end of the run.
  """
  RunCommand(command=f"rm -f {_MODPROBE_BLOCK_FILE}", check_retcode=False)


def InsertKernelModule(
    kernel_module_name: str,
    kernel_module_directory: str,
    additional_options: str = "",
) -> None:
  """Inserts the kernel module."""
  logging.info(
      "Inserting kernel module: %s",
      kernel_module_name,
  )
  kernel_module_path = os.path.join(
      kernel_module_directory, f"{kernel_module_name}.ko"
  )
  if not os.path.exists(kernel_module_path):
    raise ValueError(
        f"Kernel module does not exist at path: {kernel_module_path}"
    )
  RunCommand(command=f"insmod {kernel_module_path} {additional_options}")


def InsertMftKernelModulesIfNotLoaded(
    kernel_module_directory: str,
) -> None:
  """Loads the Mellanox Firmware Tool (MFT) kernel modules.

  The function checks if the kernel modules are loaded, and if not, it inserts
  them (mst_pci.ko and mst_pciconf.ko) from the given kernel module directory.

  Args:
    kernel_module_directory: The directory that contains the kernel modules.
  """
  if not IsKernelModuleLoaded(constants.MST_PCI_KERNEL_MODULE_NAME):
    InsertKernelModule(
        kernel_module_name=constants.MST_PCI_KERNEL_MODULE_NAME,
        kernel_module_directory=kernel_module_directory,
    )
  if not IsKernelModuleLoaded(constants.MST_PCICONF_KERNEL_MODULE_NAME):
    InsertKernelModule(
        kernel_module_name=constants.MST_PCICONF_KERNEL_MODULE_NAME,
        kernel_module_directory=kernel_module_directory,
        additional_options="debug=1",
    )


def IdentifyMftKernelModules(
    gce_instance_info: GceInstanceInfo,
) -> tuple[storage.Blob, str, str]:
  """Downloads the Mellanox Firmware Tool (MFT) kernel modules.

  Args:
    gce_instance_info: The GCE instance information.

  Returns:
    The latest MFT kernel modules blob in the COS tools bucket and the version.
  """
  # Extract the COS tools bucket name and path from the instance.
  cos_tools_bucket_name, cos_tools_bucket_path = ExtractCosToolsBucketInfo(
      gce_instance_info
  )
  glob_pattern = os.path.join(
      cos_tools_bucket_path, constants.MFT_KERNEL_MODULES_GCS_NAMING_PATTERN
  )
  logging.info(
      "Extracting MFT kernel modules from COS tools bucket gs://%s/%s with"
      " pattern: %s",
      cos_tools_bucket_name,
      cos_tools_bucket_path,
      glob_pattern,
  )
  # List the MFT kernel modules in the corresponding GCS bucket.
  mft_kernel_module_blobs = ListObjectsInGcs(
      bucket=cos_tools_bucket_name, glob_pattern=glob_pattern
  )
  if not mft_kernel_module_blobs:
    raise ValueError(
        f"No MFT kernel modules found in bucket {cos_tools_bucket_name} with"
        f" pattern {glob_pattern}"
    )

  latest_blob = None
  latest_build_version_str = None
  latest_version_str = None
  latest_version_obj = None
  version_regex = re.compile(r"mft-kernel-modules-([\d\.]+)-(\d+)-.*\.tgz$")
  for blob in mft_kernel_module_blobs:
    match = version_regex.search(blob.name)
    if not match:
      continue
    current_version_str = match.group(1)
    build_version_str = match.group(2)
    current_version_obj = version.parse(
        f"{current_version_str}.{build_version_str}"
    )
    if latest_version_obj is None or current_version_obj > latest_version_obj:
      latest_version_str = current_version_str
      latest_version_obj = current_version_obj
      latest_build_version_str = build_version_str
      latest_blob = blob
  if latest_blob is None:
    raise ValueError(
        "No valid MFT kernel modules matching pattern found in bucket"
        f" {cos_tools_bucket_name}"
    )
  logging.info(
      "The latest MFT kernel modules available on COS tools is: %s-%s",
      latest_version_str,
      latest_build_version_str,
  )
  return latest_blob, latest_version_str, latest_build_version_str


def DownloadMftKernelModulesFromCosTools(
    mft_kernel_modules_blob: storage.Blob,
) -> None:
  """Download the MFT kernel modules from COS tools bucket to local machine.

  Args:
    mft_kernel_modules_blob: The MFT kernel modules blob from the COS tools.

  Raises:
    FileNotFoundError: If the MFT kernel modules unpacked directory does not
    exist.
  """
  # Download the MFT kernel modules to local.
  mft_kernel_modules_zipped_local_path = os.path.join(
      constants.MFT_KERNEL_MODULES_LOCAL_DIR_PATH,
      os.path.basename(mft_kernel_modules_blob.name),
  )
  logging.debug(
      "Downloading MFT kernel modules from gs://%s/%s to %s",
      mft_kernel_modules_blob.bucket.name,
      mft_kernel_modules_blob.name,
      mft_kernel_modules_zipped_local_path,
  )
  TransferArtifacts(
      upload=False,
      artifact=Artifact(
          filepath=mft_kernel_modules_zipped_local_path,
          content_type="application/gzip",
      ),
      bucket=mft_kernel_modules_blob.bucket,
      gcs_path=mft_kernel_modules_blob.name,
  )

  # Unzip the MFT kernel modules.
  RunCommand(
      command=(
          f"tar -xzvf {mft_kernel_modules_zipped_local_path} -C"
          rf" {constants.MFT_KERNEL_MODULES_LOCAL_DIR_PATH}"
          r" --transform='s/.*\///'"
      )
  )


def main(argv: Sequence[str]) -> None | int:
  if len(argv) > 1:
    raise app.UsageError("Too many command-line arguments.")

  driver_userspace = os.environ.get(
      constants.DRIVER_USERSPACE_ENV,
      "not set (not started by run-goog-gpu-bug-report.sh)",
  )
  logging.info("NVIDIA driver userspace: %s", driver_userspace)
  # The toolkit can start the container yet leave out the NVIDIA tools, e.g. on
  # COS when it is not pointed at /var/lib/nvidia. Exit before doing any work,
  # so that run-goog-gpu-bug-report.sh retries with host mounts.
  if driver_userspace == constants.DRIVER_USERSPACE_NCT_RUNTIME_CDI:
    missing_tools = [
        tool for tool in constants.CDI_REQUIRED_TOOLS if not shutil.which(tool)
    ]
    if missing_tools:
      logging.error(
          "The NVIDIA Container Toolkit run lacks %s; exiting with code %d to"
          " retry with host mounts.",
          ", ".join(missing_tools),
          constants.EXIT_CDI_INCOMPLETE,
      )
      return constants.EXIT_CDI_INCOMPLETE

  gce_instance_info: GceInstanceInfo = GetGceInstanceInformationOrDie()
  gce_instance_info.driver_userspace = driver_userspace

  # Install Mellanox Firmware Tool if required.
  if IsMftNecessaryForMachineType(machine_type=gce_instance_info.machine_type):
    mft_version = constants.MFT_DEFAULT_VERSION
    mft_build_version = constants.MFT_DEFAULT_BUILD_VERSION
    mft_mode = _MFT_KERNEL_MODULES.value
    logging.info("MFT kernel module mode: %s", mft_mode)
    # Drop any block left behind by an earlier run in a reused container.
    RemoveMftKernelModulesBlock()

    if mft_mode == "never":
      # Force the driverless path even if the modules are already present.
      DisableMftKernelModules()
    # Download pre-signed MFT modules on COS hosts.
    elif IsCosHost():
      try:
        mft_kernel_modules_blob, mft_version, mft_build_version = (
            IdentifyMftKernelModules(gce_instance_info=gce_instance_info)
        )
        DownloadMftKernelModulesFromCosTools(mft_kernel_modules_blob)
        InsertMftKernelModulesIfNotLoaded(
            kernel_module_directory=constants.MFT_KERNEL_MODULES_LOCAL_DIR_PATH,
        )
      except Exception as e:  # pylint: disable=broad-exception-caught
        if mft_mode == "required":
          raise MftKernelModulesError(
              f"Failed to load MFT kernel modules on COS host: {e}"
          ) from e
        logging.warning(
            "Failed to download MFT modules from COS bucket: %s."
            " Continuing in driverless mode.",
            e,
        )
    else:
      logging.info("Host OS is non-COS. Attempting DKMS kernel compilation.")
      try:
        mft_installer_path = DownloadAndUnpackMft(
            architecture=gce_instance_info.architecture,
            mft_version=mft_version,
            mft_build_version=mft_build_version,
        )
        if not InstallMftKernelModulesViaDkms(mft_installer_path):
          if mft_mode == "required":
            raise MftKernelModulesError(
                "DKMS build/load of MFT kernel modules failed."
            )
          logging.info("DKMS build failed. Falling back to driverless mode.")
      except MftKernelModulesError:
        raise
      except Exception as e:  # pylint: disable=broad-exception-caught
        if mft_mode == "required":
          raise MftKernelModulesError(
              f"Failed to prepare MFT installer for DKMS: {e}"
          ) from e
        logging.warning("Failed to prepare MFT installer for DKMS: %s", e)

    if mft_mode == "required" and not AreMftKernelModulesLoaded():
      raise MftKernelModulesError(
          "--mft_kernel_modules=required but the MFT kernel modules are not"
          " loaded."
      )

    # Install MFT userspace software inside container for ALL OSes.
    if not IsMftInstalled():
      try:
        mft_installer_path = DownloadAndUnpackMft(
            architecture=gce_instance_info.architecture,
            mft_version=mft_version,
            mft_build_version=mft_build_version,
        )
        InstallMftUserspaceSoftware(mft_installer_path=mft_installer_path)
      except Exception as e:  # pylint: disable=broad-exception-caught
        logging.warning("Failed to install MFT userspace software: %s", e)
    # Covers MFT installed by the DKMS build or already present, not just the
    # userspace install above.
    if IsMftInstalled():
      gce_instance_info.mst_version = GetMftVersion()

    gce_instance_info.mft_kernel_modules_loaded = AreMftKernelModulesLoaded()
    logging.info(
        "MFT kernel modules loaded (driver mode): %s",
        gce_instance_info.mft_kernel_modules_loaded,
    )

  # Must run before nvidia-bug-report.sh, whose `mst gpu add` dlopens the
  # unversioned libnvidia-ml.so.
  LinkUnversionedLibraryAliases()

  if not IsNvidiaDriverInstalled():
    logging.warning(
        "NVIDIA driver (nvidia-smi) is not found on host system (driver"
        " userspace: %s).",
        driver_userspace,
    )

  timestamp = datetime.datetime.now(tz=datetime.timezone.utc).strftime(
      "utc_%Y_%m_%d_%H_%M_%S"
  )
  local_bug_report_directory = os.path.join(
      f"/tmp/goog_gpu_bug_reports/{timestamp}/vm_id_{gce_instance_info.instance_id}"
  )
  # Generate GPU VM bug report artifacts.
  try:
    artifacts = GenerateBugReport(
        gce_instance_info=gce_instance_info,
        output_directory=local_bug_report_directory,
    )
  finally:
    # nvidia-bug-report.sh (which runs `mst start`) is done; lift the block.
    RemoveMftKernelModulesBlock()

  # Create a single master .tar.gz bundle containing all collected logs.
  master_archive = CreateMasterBugReportArchive(
      artifacts=artifacts,
      output_directory=local_bug_report_directory,
      instance_id=gce_instance_info.instance_id,
      timestamp=timestamp,
  )

  logging.info(gce_instance_info)
  logging.info(
      "Bug report logs are available locally at: %s",
      local_bug_report_directory,
  )

  logging.info(
      "Master bug report archive created at: %s",
      master_archive.filepath,
  )

  if _GCS_BUCKET.value:
    gcs_destination_directory = (
        f"bug_report/{timestamp}/vm_id_{gce_instance_info.instance_id}"
    )

    # Upload master archive to GCS.
    UploadArtifactsToGcs(
        artifacts_to_upload=[master_archive],
        bucket_name=_GCS_BUCKET.value,
        destination_directory=gcs_destination_directory,
    )

    logging.info(
        "Bug report master archive is available at: %s",
        f"https://console.cloud.google.com/storage/browser/{os.path.join(_GCS_BUCKET.value, gcs_destination_directory)}",
    )


if __name__ == "__main__":
  app.run(main)
