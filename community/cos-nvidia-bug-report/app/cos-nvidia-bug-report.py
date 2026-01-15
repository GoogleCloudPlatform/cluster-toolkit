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
import subprocess

from absl import app
from absl import flags
from absl import logging
import constants
from google.cloud import exceptions as google_cloud_exceptions
from google.cloud import storage
from packaging import version
import requests
import retry

_GCS_BUCKET = flags.DEFINE_string(
    "gcs_bucket",
    None,
    "The GCS bucket to upload the bug report to.",
)


@dataclasses.dataclass
class Artifact:
  """Dataclass to hold generated artifact."""

  filepath: str
  content_type: str = "application/octet-stream"


@dataclasses.dataclass
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
  )
  logging.debug("Running command: %s", command)
  try:
    stdout, stderr = process.communicate(timeout=timeout_sec)
  except subprocess.TimeoutExpired as te:
    # clean up the process before raising the timeout experied.
    process.kill()
    raise te
  stdout = stdout.decode()
  stderr = stderr.decode()
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


def GetArchitectureFromMachineType(machine_type: str) -> Architecture:
  """Returns the architecture of the machine type.

  Args:
    machine_type: The machine type to get the architecture for.

  Returns:
    The architecture of the machine type. This can be either X86 or ARM.
  """
  machine_type = machine_type.lower().split("-")
  if machine_type[0].lower().endswith("x"):
    return Architecture.ARM
  else:
    return Architecture.X86


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
  response = requests.get(metadata_url, headers=constants.METADATA_HEADERS)
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

  architecture = GetArchitectureFromMachineType(machine_type)

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

  bucket = storage_client.lookup_bucket(bucket_name)
  if bucket is None:
    # Create the bucket if it does not exist.
    try:
      storage_client.create_bucket(bucket)
      logging.info("Created bucket [%s] in the project.", bucket_name)
    except google_cloud_exceptions.exceptions.GoogleAPIError as e:
      logging.info(
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


def GenerateBugReport(
    gce_instance_info: GceInstanceInfo,
    output_directory: str = "/tmp/nvidia_bug_reports",
) -> Sequence[Artifact]:
  """Generates a bug report at the given output directory.

  The function invokes nvidia-bug-report script to generate a bug report on the
  GPUs, it contains the dmesg logs, PCI tree topology, NVLink logs, etc.

  Additionally, the script creates a instance_info.txt file that contains the
  GCE instance information.

  Args:
    gce_instance_info: The GCE instance information.
    output_directory: The directory to store the bug report locally.

  Returns:
    A sequence of artifacts generated by the NVIDIA bug report script.
  """
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
      command=f"chmod 777  {constants.DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH}"
  )
  # Generate the NVIDIA bug report.
  RunCommand(
      command=(
          f"{constants.DUPLICATED_NVIDIA_BUG_REPORT_SCRIPT_PATH} {constants.NVIDIA_BUG_REPORT_FLAGS}"
      ),
      timeout_sec=600,
  )
  final_nvidia_bug_report_output_path = os.path.join(
      output_directory,
      constants.NVIDIA_BUG_REPORT_OUTPUT_NAME,
  )
  shutil.move(
      default_nvidia_bug_report_output_path,
      final_nvidia_bug_report_output_path,
  )
  final_instance_info_path = os.path.join(
      output_directory,
      "instance_info.txt",
  )
  with open(final_instance_info_path, "w") as file_handle:
    file_handle.write(str(gce_instance_info))

  artifacts = [
      Artifact(
          filepath=final_nvidia_bug_report_output_path,
          content_type="application/gzip",
      ),
      Artifact(
          filepath=final_instance_info_path,
          content_type="text/plain",
      ),
  ]
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

  response = requests.get(url)
  response.raise_for_status()
  with open(destination_path, "wb") as file_handle:
    for chunk in response.iter_content(chunk_size=8192):
      file_handle.write(chunk)
  logging.info("Downloaded file to: %s", destination_path)


def InstallMftUserspaceSoftware(
    architecture: Architecture, mft_version: str, mft_build_version: str
) -> None:
  """Installs Mellanox Firmware Tool (MFT) userspace software on the instance.

  Args:
    architecture: The architecture of the GCE instance. This can be either X86
      or ARM.
    mft_version: The version of the MFT to install.
    mft_build_version: The build version of the MFT to install.

  Raises:
    ValueError: If the architecture is not supported.
  """
  if architecture != Architecture.X86 and architecture != Architecture.ARM:
    raise ValueError(f"Unsupported architecture: {architecture}")
  logging.info(
      "Installing MFT userspace software for architecture: %s",
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
  unpacked_mft_directory_path = mft_filename_path.replace(".tgz", "")
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
  logging.debug("Installing MFT through installer: %s", mft_installer_path)
  RunCommand(
      command=f"bash {mft_installer_path} --without-kernel",
      timeout_sec=600,
  )
  logging.info("MFT installation completed.")


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

  The function checks the COS release info file at /etc/lsb-release on the host,
  (passed in as /etc_host/lsb-release in the docker container) and extracts the
  closest COS tools that matches the COS release information and the physical
  zone of the instance.

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
  """Checks if the running MFT software is necessary for the current VM type.

  Args:
    machine_type: The machine type of the VM.

  Returns:
    True if the Mellanox devices is supposed to be included in the VM's
    underlying hardware, False otherwise. GCE uses Mellanox devices (CX7 NICs)
    on machines for A4 VM.
  """
  return machine_type.startswith("a4")


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
):
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
          f" {constants.MFT_KERNEL_MODULES_LOCAL_DIR_PATH} --transform='s/.*\///'"
      )
  )


def main(argv: Sequence[str]) -> None:
  if len(argv) > 1:
    raise app.UsageError("Too many command-line arguments.")

  gce_instance_info: GceInstanceInfo = GetGceInstanceInformationOrDie()

  # Install Mellanox Firmware Tool if it is not already installed.
  if IsMftNecessaryForMachineType(machine_type=gce_instance_info.machine_type):
    # Identify the latest MFT kernel modules available from the COS tools bucket
    mft_kernel_modules_blob, mft_version, mft_build_version = (
        IdentifyMftKernelModules(gce_instance_info=gce_instance_info)
    )
    DownloadMftKernelModulesFromCosTools(mft_kernel_modules_blob)
    # Insert the MFT kernel modules if they are not already loaded.
    InsertMftKernelModulesIfNotLoaded(
        kernel_module_directory=constants.MFT_KERNEL_MODULES_LOCAL_DIR_PATH,
    )
    # Install MFT userspace program if it is not already installed.
    if not IsMftInstalled():
      InstallMftUserspaceSoftware(
          architecture=gce_instance_info.architecture,
          mft_version=mft_version,
          mft_build_version=mft_build_version,
      )
    gce_instance_info.mst_version = GetMftVersion()

  if not IsNvidiaDriverInstalled():
    raise ValueError(
        "NVIDIA driver is not installed. Please install the NVIDIA driver"
        " before running this script. You can confirm whether the driver is"
        " installed by running the command: which nvidia-smi"
    )

  timestamp = datetime.datetime.now(tz=datetime.timezone.utc).strftime(
      "utc_%Y_%m_%d_%H_%M_%S"
  )
  local_nvidia_bug_report_directory = os.path.join(
      f"/tmp/nvidia_bug_reports/{timestamp}/vm_id_{gce_instance_info.instance_id}"
  )
  # Generate an NVIDIA bug report.
  artifacts = GenerateBugReport(
      gce_instance_info=gce_instance_info,
      output_directory=local_nvidia_bug_report_directory,
  )

  logging.info(gce_instance_info)
  logging.info(
      "Bug report logs are available locally at: %s",
      local_nvidia_bug_report_directory,
  )

  if _GCS_BUCKET.value:
    gcs_destination_directory = (
        f"bug_report/{timestamp}/vm_id_{gce_instance_info.instance_id}"
    )

    # Upload artifacts to GCS.
    UploadArtifactsToGcs(
        artifacts_to_upload=artifacts,
        bucket_name=_GCS_BUCKET.value,
        destination_directory=gcs_destination_directory,
    )

    logging.info(
        "Bug report logs are available at: %s",
        f"https://pantheon.corp.google.com/storage/browser/{os.path.join(_GCS_BUCKET.value, gcs_destination_directory)}",
    )

if __name__ == "__main__":
  app.run(main)
