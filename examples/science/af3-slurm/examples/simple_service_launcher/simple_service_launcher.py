# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import logging.handlers
from google.cloud import storage
import os
import argparse
import subprocess
import datetime
import re
import logging
import time
import uuid
import json
import yaml
import collections
from typing import Generator, Optional, Tuple, Dict, Any, List
import dataclasses
import shutil 

# --- Constants and Basic Setup ---
HOME_DIR = os.getenv("HOME", os.path.expanduser("~"))
UNIQUE_ID = f"{str(uuid.uuid4())[:8]}"
LOG_DIR = (
    f"{HOME_DIR}/af3_service_log_{datetime.datetime.now().strftime('%Y-%m-%d-%H-%M')}-{UNIQUE_ID}"
)
os.makedirs(LOG_DIR, mode=0o777, exist_ok=True)

# --- Logging Setup ---
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(levelname)s - %(message)s",
    handlers=[
        logging.handlers.RotatingFileHandler(
            os.path.join(LOG_DIR, "app.log"), maxBytes=10 * 1024 * 1024, backupCount=3
        ),
        logging.StreamHandler(),
    ],
)
logger = logging.getLogger(__name__)

# --- Dataclasses ---
@dataclasses.dataclass
class ServiceConfig:
    time_interval: int
    run_data_pipeline: bool = True
    run_inference: bool = True
    pending_job_time_limit: Optional[int] = None

@dataclasses.dataclass
class BaseProcessConfig:
    bucket_input_dir: str
    bucket_running_dir: str
    bucket_done_dir: str
    bucket_failed_dir: str
    bucket_result_dir: str
    local_work_dir: str
    partition_name: str
    job_memory_size: int
    job_cpu_count: int
    job_timeout: int

# Define common argument names/properties shared between data-pipeline and inference
# Format: (config_key, arg_suffix, type, help_suffix)
COMMON_PROCESS_ARGS_DEF = [
    ("bucket_input_dir", "bucket-input-dir", str, "input files"),
    ("bucket_running_dir", "bucket-running-dir", str, "running jobs"),
    ("bucket_done_dir", "bucket-done-dir", str, "successful jobs"),
    ("bucket_failed_dir", "bucket-failed-dir", str, "failed jobs"),
    ("bucket_result_dir", "bucket-result-dir", str, "storing results"),
    ("job_timeout", "job-timeout", int, "timeout (seconds)"),
    ("partition_name", "partition-name", str, "partition name"),
    ("job_memory_size", "job-memory-size", int, "job memory size (GB)"),
    ("job_cpu_count", "job-cpu-count", int, "job cpu count"),
    ("local_work_dir", "local-dir", str, "local working directory"),
]

@dataclasses.dataclass
class ProcessConfig(BaseProcessConfig):
    bucket_name: str
    sif_dir: str
    db_dir: str
    model_dir: str
    is_inference: bool = False
    # Process specific optional args
    bucket_submit_inference_dir: Optional[str] = None # For data pipeline
    pdb_database_path: Optional[str] = None          # For data pipeline
    jax_compilation_cache_path: Optional[str] = None # For inference
    # Inference specific optional args
    max_template_date: Optional[str] = None
    conformer_max_iterations: Optional[int] = None
    num_recycles: Optional[int] = None
    num_diffusion_samples: Optional[int] = None
    num_seeds: Optional[int] = None
    save_embeddings: Optional[bool] = None

@dataclasses.dataclass
class SlurmJobWorkspaceConfig:
    file_name_without_extension: str
    base_dir: str
    running_dir: str
    result_dir: str
    script_dir: str
    log_dir: str

@dataclasses.dataclass
class SlurmJobInfo:
    job_id: str
    bucket_running_file_path: str # Full gs:// path with job id
    bucket_fail_file_path: str    # Full gs:// path

# --- Helper Functions ---

def setup_slurm_job_workspace(
    config: ProcessConfig, timestamp: str, blobname: str
) -> SlurmJobWorkspaceConfig:
    """Setup SLURM job related directories."""
    # Use filename base from blob for uniqueness, handle potential nesting
    blob_basename = os.path.basename(blobname)
    file_name_without_extension = os.path.splitext(blob_basename)[0]
    # Construct base directory path using timestamp and unique filename base
    base_dir = os.path.join(config.local_work_dir, timestamp, file_name_without_extension)

    local_result_dir = os.path.join(base_dir, "result")
    local_running_dir = os.path.join(base_dir, "running") # Holds downloaded input
    local_slurm_script_dir = os.path.join(base_dir, "scripts", config.partition_name)
    local_slurm_log_dir = os.path.join(base_dir, "slurmlog") # Slurm logs go here

    # Create directories with exist_ok=True
    for dir_path in [local_result_dir, local_running_dir, local_slurm_log_dir, local_slurm_script_dir]:
        try:
            os.makedirs(dir_path, mode=0o777, exist_ok=True) # Added mode for clarity
            logger.debug(f"Ensured directory exists: {dir_path}")
        except OSError as e:
            logger.error(f"Failed to create directory {dir_path}: {e}")
            raise # Re-raise directory creation error

    return SlurmJobWorkspaceConfig(
        file_name_without_extension=file_name_without_extension,
        base_dir=base_dir,
        running_dir=local_running_dir,
        result_dir=local_result_dir,
        script_dir=local_slurm_script_dir,
        log_dir=local_slurm_log_dir,
    )

def load_config(config_file_path: str) -> dict:
    """Load configuration data from a JSON or YAML file."""
    _, ext = os.path.splitext(config_file_path)
    try:
        with open(config_file_path, "r", encoding="utf-8") as file:
            if ext == ".json":
                return json.load(file)
            elif ext in (".yaml", ".yml"):
                return yaml.safe_load(file)
            raise ValueError(f"Unsupported configuration file format: {ext}")
    except FileNotFoundError:
        logger.error(f"Configuration file not found: {config_file_path}")
        raise
    except Exception as e:
        logger.error(f"Error loading configuration file {config_file_path}: {e}")
        raise

def listing_bucket_dir(
    client: storage.Client, bucket_name: str, source_dir: str
) -> Generator[str, None, None]:
    """List all blob names under a GCS prefix, excluding folders."""
    bucket = client.get_bucket(bucket_name)
    # Ensure prefix ends with '/' for correct listing
    prefix = source_dir if source_dir.endswith('/') else source_dir + '/'
    blobs = bucket.list_blobs(prefix=prefix)
    logger.debug(f"Listing blobs in gs://{bucket_name}/{prefix}")
    count = 0
    for blob in blobs:
        if not blob.name.endswith('/'): # Exclude directory placeholders
             # Yield relative path from bucket root
             yield blob.name
             count += 1
    logger.debug(f"Listing finished for gs://{bucket_name}/{prefix}. Found {count} potential items.")

def move_bucket_file(
    client: storage.Client,
    bucket_name: str,
    source_blob_name_rel: str, # Relative path within bucket
    destination_blob_name_rel: str, # Relative path within bucket
) -> None:
    """Move a blob within a GCS bucket using relative paths."""
    logger.debug(f"Moving gs://{bucket_name}/{source_blob_name_rel} to gs://{bucket_name}/{destination_blob_name_rel}")
    try:
        source_bucket = client.bucket(bucket_name)
        source_blob = source_bucket.blob(source_blob_name_rel)
        # Check if blob exists before copy/delete
        if not source_blob.exists():
             logger.warning(f"Source blob gs://{bucket_name}/{source_blob_name_rel} not found for move.")
             # Depending on use case, maybe raise error or just return
             raise FileNotFoundError(f"Source blob not found: gs://{bucket_name}/{source_blob_name_rel}")

        _ = source_bucket.copy_blob(source_blob, source_bucket, destination_blob_name_rel)
        source_blob.delete() # Delete only after successful copy
        logger.debug(f"Move successful.")
    except Exception as e:
        logger.error(f"Error moving blob from {source_blob_name_rel} to {destination_blob_name_rel}: {e}")
        raise # Re-raise to signal failure

def run_command(command: List[str]) -> Optional[str]:
    """Execute a command and return stdout, handling errors."""
    try:
        command_str = ' '.join(command) # For logging
        logger.debug(f"Running command: {command_str}")
        result = subprocess.run(command, capture_output=True, text=True, check=True, encoding='utf-8')
        # Log truncated stdout only if needed, maybe just on success info level
        logger.debug(f"Command finished successfully.") # Simplify debug log
        return result.stdout
    except subprocess.CalledProcessError as e:
        logger.error(f"Command failed: {command_str}. Return code: {e.returncode}. Stderr: {e.stderr.strip()}")
        raise RuntimeError(f"Command failed with stderr: {e.stderr.strip()}")
    except FileNotFoundError:
         logger.error(f"Command not found: {command[0]}")
         raise RuntimeError(f"Command not found: {command[0]}")
    except Exception as e:
        logger.error(f"Unexpected error running command {command_str}: {e}")
        raise

def str_to_bool(value: Any) -> bool:
    """Convert string/bool/int to boolean, raising ArgumentTypeError on failure."""
    if isinstance(value, bool): return value
    if isinstance(value, str):
        val_lower = value.lower()
        if val_lower in ("true", "1", "t", "y", "yes"): return True
        if val_lower in ("false", "0", "f", "n", "no"): return False
    elif isinstance(value, int): return value != 0
    raise argparse.ArgumentTypeError(f"Boolean value expected, got '{value}' (type: {type(value)})")

def parse_arguments(parser: argparse.ArgumentParser) -> argparse.Namespace:
    """Parse command-line arguments."""
    parser.add_argument("--bucket-name", type=str, default=None, help="GCS bucket name (required if not in config)")
    parser.add_argument("--config-file", type=str, help="Configuration file (json/yaml)")

    # Add common arguments for both processes
    for process_type in ["data-pipeline", "inference"]:
        process_prefix_ = process_type.replace('-', '_')
        for conf_key, arg_suffix, arg_type, help_suffix in COMMON_PROCESS_ARGS_DEF:
             parser.add_argument(
                 f"--{process_type}-{arg_suffix}",
                 dest=f"{process_prefix_}_{conf_key}", # e.g., data_pipeline_job_timeout
                 type=arg_type, required=False,
                 help=f"{process_type.capitalize()}: {help_suffix}",
             )
        # Add run flag separately
        parser.add_argument(
            f"--run-{process_type}", dest=f"run_{process_prefix_}",
            type=str_to_bool, required=False, nargs='?', const=True, # Allows --run-X (True) or --run-X false
            help=f"Enable checking/running {process_type} jobs",
        )

    # Add specific/global arguments
    parser.add_argument("--pdb-database-path", type=str, help="Data Pipeline: Path to PDB database")
    parser.add_argument("--jax-compilation-cache-path", type=str, help="Inference: Path to JAX cache")
    parser.add_argument("--sif-dir", type=str, help="Global: Directory for Singularity images")
    parser.add_argument("--db-dir", type=str, help="Global: Directory for databases")
    parser.add_argument("--model-dir", type=str, help="Global: Directory for model files")
    parser.add_argument("--time-interval", type=int, help="Global: Service check interval (seconds)")
    parser.add_argument("--pending-job-time-limit", type=int, help="Global: Max time (seconds) a job can be PENDING")

    # Add inference-specific arguments
    parser.add_argument("--inference-max-template-date", type=str, help="Inference: Max template date (YYYY-MM-DD)")
    parser.add_argument("--inference-conformer-max-iterations", type=int, help="Inference: Max conformer iterations")
    parser.add_argument("--inference-num-recycles", type=int, help="Inference: Number of recycles")
    parser.add_argument("--inference-num-diffusion-samples", type=int, help="Inference: Number of diffusion samples")
    parser.add_argument("--inference-num-seeds", type=int, help="Inference: Number of seeds")
    # Use store_true/store_false or handle bool conversion carefully
    parser.add_argument("--inference-save-embeddings", type=str_to_bool, nargs='?', const=True, default=None, help="Inference: Save embeddings")

    return parser.parse_args()

def convert_seconds_to_hms(seconds: int) -> str:
    """Convert seconds to HH:MM:SS format."""
    if not isinstance(seconds, int) or seconds < 0:
         logger.warning(f"Invalid input for time conversion: {seconds}. Returning 00:00:00")
         return "00:00:00"
    hours, remainder = divmod(seconds, 3600)
    minutes, secs = divmod(remainder, 60)
    return f"{hours:02}:{minutes:02}:{secs:02}"

def search_regex_pattern(pattern: str, string: str) -> Optional[str]:
    """Search for regex pattern and return first group, or None."""
    match = re.search(pattern, string)
    return match.group(1) if match else None

def get_job_state_scontrol(job_id: str) -> Optional[str]:
    """Retrieve SLURM job state using scontrol."""
    try:
        output = run_command(["scontrol", "show", "job", "-o", job_id])
        if output is None: return None # Command failed or no output
        # Parse key=value pairs
        job_info = dict(pair.split("=", 1) for pair in output.split() if "=" in pair)
        state = job_info.get("JobState")
        if state: return state
        # Fallback if JobState isn't present (e.g., completed job)
        logger.warning(f"JobState not found directly in scontrol output for {job_id}. Checking Reason.")
        reason = job_info.get("Reason")
        if reason and reason != "None":
             if "COMPLETED" in reason: return "COMPLETED"
             if "CANCELLED" in reason: return "CANCELLED"
             if "FAILED" in reason: return "FAILED"
             if "TIMEOUT" in reason: return "TIMEOUT"
        logger.warning(f"Could not determine definitive state for job {job_id} from scontrol. Treating as UNKNOWN/PURGED.")
        return "UNKNOWN_OR_PURGED" # Treat as completed/failed if state unclear
    except RuntimeError as e:
        if "Invalid job id specified" in str(e):
            logger.warning(f"Job {job_id} not found by scontrol (likely completed/purged).")
            return "UNKNOWN_OR_PURGED"
        else: # Includes other scontrol command failures
            logger.error(f"scontrol command failed for job {job_id}: {e}")
            return None # Indicate failure to get state
    except Exception as e:
         logger.error(f"Unexpected error getting job state for {job_id}: {e}")
         return None

# --- SLURM Script Generation Helpers ---

def _generate_sbatch_header(
    process_config: ProcessConfig, slurm_job_workspace_config: SlurmJobWorkspaceConfig, use_gpu: bool
) -> str:
    """Generates the #SBATCH directives."""
    job_name = slurm_job_workspace_config.file_name_without_extension
    log_dir = slurm_job_workspace_config.log_dir # e.g., /path/to/base/slurmlog
    log_output_path = os.path.join(log_dir, "job_%j", "out.txt") # Relative path for Slurm
    log_error_path = os.path.join(log_dir, "job_%j", "err.txt") # Relative path for Slurm

    headers = [
        f"#SBATCH --job-name={job_name}",
        f"#SBATCH --partition={process_config.partition_name}",
        f"#SBATCH --time={convert_seconds_to_hms(process_config.job_timeout)}",
        f"#SBATCH --mem={process_config.job_memory_size}G",
        f"#SBATCH --cpus-per-task={process_config.job_cpu_count}",
        f"#SBATCH --output={log_output_path}",
        f"#SBATCH --error={log_error_path}",
    ]
    if use_gpu:
        headers.append("#SBATCH --gres=gpu:1")
    return "\n".join(headers)

def _build_alphafold_command(
    base_command: List[str],
    fixed_args: Dict[str, Any],
    optional_args_config: Dict[str, Optional[Any]],
    bool_flags: List[str]
    ) -> str:
    """Builds the `run_alphafold.py` command line string."""
    command_parts = base_command[:]
    all_args = {**fixed_args, **optional_args_config} # Combine args

    for key, value in all_args.items():
        if value is not None: # Only add if value is provided
            if key in bool_flags:
                # Handle boolean flags: append if True, assumes --flag=true format
                command_parts.append(f"--{key}={str(value).lower()}")
            else:
                 command_parts.append(f"--{key}={value}")

    # Escape parts if necessary, although paths usually don't need it here
    return " ".join(command_parts)


def _generate_slurm_script_content(
    process_config: ProcessConfig,
    slurm_job_workspace_config: SlurmJobWorkspaceConfig,
    sbatch_header: str,
    apptainer_command: str,
    af3_command: str,
    bucket_running_file_path_template: str, # Template with $SLURM_JOB_ID
    bucket_failed_dir: str, # Full gs:// path ending with /jobname/
    bucket_success_dir: str, # Full gs:// path ending with /jobname/
    bucket_result_dir: str, # Full gs:// path ending with /jobname/
    post_success_commands: Optional[List[str]] = None
) -> str:
    """
    Generates the bash script content for SLURM.

    Modifications:
    - Keeps SLURM logs locally on success by default.
    - Selectively removes only the local result directory upon full success.
    """
    # Use absolute paths for clarity inside the script
    local_result_dir_abs = slurm_job_workspace_config.result_dir
    base_dir_abs = slurm_job_workspace_config.base_dir
    # Define local log path structure for potential GCS copy (and debugging echo)
    local_log_dir_for_job = os.path.join(slurm_job_workspace_config.log_dir, "job_$SLURM_JOB_ID")

    script_template = f"""#!/bin/bash
{sbatch_header}

echo "==================== SLURM JOB START ===================="
echo "Job ID         : $SLURM_JOB_ID"
echo "Job Name       : $SLURM_JOB_NAME"
echo "Executing on   : $(hostname)"
echo "Working dir    : $(pwd)" # Should be sif_dir after cd
echo "Workspace      : {base_dir_abs}"
echo "Result Dir     : {local_result_dir_abs}"
echo "Log Dir (local): {slurm_job_workspace_config.log_dir}/job_$SLURM_JOB_ID" # Show expected local log path
echo "Timestamp      : $(date)"
echo "-------------------- Apptainer Command --------------------"
echo "{apptainer_command}"
echo "-------------------- AlphaFold Command --------------------"
echo "{af3_command}"
echo "-----------------------------------------------------------"

cd "{process_config.sif_dir}" || {{ echo "ERROR: Failed to cd to {process_config.sif_dir}"; exit 1; }}

# Execute the main commands
{apptainer_command} \\
    {af3_command}

job_exit_code=$?

# Define final GCS paths using SLURM_JOB_ID
# Note: bucket_*_dir args already contain gs:// prefix (ending with /jobname/)
BUCKET_RUNNING_PATH=$(echo "{bucket_running_file_path_template}" | sed "s/\\$SLURM_JOB_ID/$SLURM_JOB_ID/g")

if [ $job_exit_code -ne 0 ]; then
  echo "!!!!!!!!!!!!!!!!!!!! JOB FAILED !!!!!!!!!!!!!!!!!!!!"
  echo "Slurm job $SLURM_JOB_ID failed with exit code $job_exit_code"
  echo "Moving input marker from $BUCKET_RUNNING_PATH to {bucket_failed_dir}"
  gcloud storage mv "$BUCKET_RUNNING_PATH" "{bucket_failed_dir}" --quiet || echo "Warning: Failed to move input marker to failed dir."

  # --- OPTIONAL: Copy Logs on Failure ---
  # Uncomment the following block if you want logs copied to GCS on failure
  # if [ -d "{local_log_dir_for_job}" ]; then # Check if log dir exists locally
  #     echo "Attempting to copy SLURM logs from {local_log_dir_for_job} to failure directory: {bucket_failed_dir}slurm_logs/"
  #     gcloud storage cp -r "{local_log_dir_for_job}/" "{bucket_failed_dir}slurm_logs/" --quiet || echo "Warning: Failed to copy SLURM logs to failure directory {bucket_failed_dir}slurm_logs/."
  # else
  #     echo "Warning: Local SLURM log directory {local_log_dir_for_job} not found for copying on failure."
  # fi
  # --- End OPTIONAL Copy Logs on Failure ---

  echo "Job failed. Local workspace and logs kept for debugging at: {base_dir_abs}" # Workspace kept on failure
  echo "==================== SLURM JOB END (FAILED) ===================="
  exit $job_exit_code
fi

# --- Success Path ---
echo "******************** JOB SUCCEEDED ********************"
echo "Slurm job $SLURM_JOB_ID completed successfully."
echo "Copying results from {local_result_dir_abs} to {bucket_result_dir}"
gcloud storage cp -r "{local_result_dir_abs}/"* "{bucket_result_dir}" --quiet # Copy contents quietly
copy_exit_code=$?
if [ $copy_exit_code -ne 0 ]; then
    echo "!!!!!!!!!!!!!!!!!!!! RESULT COPY FAILED !!!!!!!!!!!!!!!!!!!!"
    echo "ERROR: gcloud storage cp failed with exit code $copy_exit_code."
    echo "Attempting to move input marker to failed dir: {bucket_failed_dir}"
    gcloud storage mv "$BUCKET_RUNNING_PATH" "{bucket_failed_dir}" --quiet || echo "Warning: Failed to move input marker to failed dir after copy failure."

    # --- OPTIONAL: Copy Logs on Result Copy Failure ---
    # Uncomment the following block if you want logs copied to GCS on copy failure
    # if [ -d "{local_log_dir_for_job}" ]; then # Check if log dir exists locally
    #     echo "Attempting to copy SLURM logs from {local_log_dir_for_job} to failure directory: {bucket_failed_dir}slurm_logs/"
    #     gcloud storage cp -r "{local_log_dir_for_job}/" "{bucket_failed_dir}slurm_logs/" --quiet || echo "Warning: Failed to copy SLURM logs to failure directory {bucket_failed_dir}slurm_logs/ after result copy failure."
    # else
    #     echo "Warning: Local SLURM log directory {local_log_dir_for_job} not found for copying on result copy failure."
    # fi
    # --- End OPTIONAL Copy Logs on Result Copy Failure ---

    echo "Result copy failed. Local workspace and logs kept for debugging at: {base_dir_abs}" # Workspace kept on copy failure
    echo "================ SLURM JOB END (COPY FAILED) ================"
    exit $copy_exit_code # Exit with copy error code
fi
echo "Results successfully copied to GCS."

# Optional post-success commands (like copying to inference input)
{chr(10).join(post_success_commands) if post_success_commands else ''}

echo "Moving input marker from $BUCKET_RUNNING_PATH to {bucket_success_dir}"
gcloud storage mv "$BUCKET_RUNNING_PATH" "{bucket_success_dir}" --quiet || echo "Warning: Failed to move input marker to success dir."


# --- Cleanup on Full Success ---
# Deletes only the local result directory, keeping logs, scripts, input copy locally.
echo "Cleaning up local result directory ONLY: {local_result_dir_abs}"
rm -rf "{local_result_dir_abs}"
cleanup_exit_code=$?
if [ $cleanup_exit_code -ne 0 ]; then
    echo "Warning: Failed to clean up local result directory {local_result_dir_abs}. Exit code: $cleanup_exit_code"
fi
# NOTE: The base_dir, slurmlog, scripts, running directories remain locally with this approach.


echo "Job finished successfully."
echo "==================== SLURM JOB END (SUCCESS) ===================="
"""
    # Use strip() to remove leading/trailing whitespace from the template
    return script_template.strip()

# --- Main Process Functions ---

def generate_data_pipeline_slurm_script(
    process_config: ProcessConfig,
    slurm_job_workspace_config: SlurmJobWorkspaceConfig,
    timestamp: str,
    local_input_file_path: str,
) -> Tuple[str, str]:
    """Generate SLURM script for data pipeline."""
    file_name_without_extension = slurm_job_workspace_config.file_name_without_extension
    file_name = os.path.basename(local_input_file_path)
    gcs_base = f"gs://{process_config.bucket_name}"

    # Define GCS paths (ensure trailing slashes for directories)
    running_path_template = f"{gcs_base}/{process_config.bucket_running_dir}{file_name_without_extension}/{timestamp}/$SLURM_JOB_ID/{file_name}"
    failed_dir_gcs = f"{gcs_base}/{process_config.bucket_failed_dir}{file_name_without_extension}/{timestamp}/"
    success_dir_gcs = f"{gcs_base}/{process_config.bucket_done_dir}{file_name_without_extension}/{timestamp}/"
    result_dir_gcs = f"{gcs_base}/{process_config.bucket_result_dir}{file_name_without_extension}/{timestamp}/"

    # Build Apptainer command
    apptainer_binds = [ f"--bind {p}:{p}" for p in [slurm_job_workspace_config.result_dir, process_config.db_dir] if p]
    if process_config.pdb_database_path: apptainer_binds.append(f"--bind {process_config.pdb_database_path}:{process_config.pdb_database_path}")
    apptainer_command = f"apptainer run --nv {' '.join(apptainer_binds)}"

    # Build AlphaFold command
    af3_fixed_args = {
        "json_path": local_input_file_path, "output_dir": slurm_job_workspace_config.result_dir,
        "run_data_pipeline": "True", "run_inference": "False", "db_dir": process_config.db_dir,
        "jackhmmer_n_cpu": process_config.job_cpu_count, "nhmmer_n_cpu": process_config.job_cpu_count,
    }
    af3_optional_args = {"pdb_database_path": process_config.pdb_database_path}
    af3_command = _build_alphafold_command(
        ["af3.sif", "python3", "/app/alphafold/run_alphafold.py"], af3_fixed_args, af3_optional_args, []
    )

    # Post-success command: Copy results to inference input
    post_success = []
    if process_config.bucket_submit_inference_dir:
        inference_input_dir = f"{gcs_base}/{process_config.bucket_submit_inference_dir}{file_name_without_extension}/{timestamp}/"
        post_success.append(f'echo "Copying results to inference input: {inference_input_dir}"')
        post_success.append(f'gcloud storage cp -r "{slurm_job_workspace_config.result_dir}/"* "{inference_input_dir}" --quiet || echo "Warning: Failed to copy results to inference input dir."')

    # Generate script content
    sbatch_header = _generate_sbatch_header(process_config, slurm_job_workspace_config, use_gpu=False)
    script_content = _generate_slurm_script_content(
        process_config, slurm_job_workspace_config, sbatch_header,
        apptainer_command, af3_command, running_path_template,
        failed_dir_gcs, success_dir_gcs, result_dir_gcs, post_success
    )

    # Write script file
    script_path = os.path.join(slurm_job_workspace_config.script_dir, f"{file_name_without_extension}.sh")
    logger.debug(f"Writing data pipeline SLURM script to {script_path}")
    try:
        with open(script_path, "w", encoding="utf-8") as f: f.write(script_content)
        os.chmod(script_path, 0o755) # Ensure script is executable
    except IOError as e:
        logger.error(f"Failed to write or chmod script file {script_path}: {e}")
        raise

    return script_path, running_path_template # Return template for job submission step

def generate_inference_slurm_script(
    process_config: ProcessConfig,
    slurm_job_workspace_config: SlurmJobWorkspaceConfig,
    timestamp: str,
    local_input_file_path: str,
) -> Tuple[str, str]:
    """Generate SLURM script for inference."""
    file_name = os.path.basename(local_input_file_path)
    gcs_base = f"gs://{process_config.bucket_name}"
    # Infer name for grouping results (heuristic based on input path)
    data_pipeline_name = os.path.basename(os.path.dirname(local_input_file_path))
    if not data_pipeline_name or data_pipeline_name == "running": # Fallback
        data_pipeline_name = slurm_job_workspace_config.file_name_without_extension
        logger.debug(f"Using fallback name for result grouping: {data_pipeline_name}")

    # Define GCS paths
    running_path_template = f"{gcs_base}/{process_config.bucket_running_dir}{data_pipeline_name}/{timestamp}/$SLURM_JOB_ID/{file_name}"
    failed_dir_gcs = f"{gcs_base}/{process_config.bucket_failed_dir}{data_pipeline_name}/{timestamp}/"
    success_dir_gcs = f"{gcs_base}/{process_config.bucket_done_dir}{data_pipeline_name}/{timestamp}/"
    result_dir_gcs = f"{gcs_base}/{process_config.bucket_result_dir}{data_pipeline_name}/{timestamp}/"

    # Build Apptainer command
    apptainer_binds = [ f"--bind {p}:{p}" for p in [slurm_job_workspace_config.result_dir, process_config.model_dir] if p]
    if process_config.jax_compilation_cache_path: apptainer_binds.append(f"--bind {process_config.jax_compilation_cache_path}:{process_config.jax_compilation_cache_path}")
    apptainer_command = f"apptainer run --nv {' '.join(apptainer_binds)}"

    # Build AlphaFold command
    af3_fixed_args = {
        "json_path": local_input_file_path, "output_dir": slurm_job_workspace_config.result_dir,
        "run_data_pipeline": "False", "run_inference": "True", "model_dir": process_config.model_dir,
    }
    af3_optional_args = {
        "jax_compilation_cache_dir": process_config.jax_compilation_cache_path,
        "max_template_date": process_config.max_template_date,
        "conformer_max_iterations": process_config.conformer_max_iterations,
        "num_recycles": process_config.num_recycles,
        "num_diffusion_samples": process_config.num_diffusion_samples,
        "num_seeds": process_config.num_seeds,
        "save_embeddings": process_config.save_embeddings,
    }
    af3_command = _build_alphafold_command(
        ["af3.sif", "python3", "/app/alphafold/run_alphafold.py"],
        af3_fixed_args, af3_optional_args, ["save_embeddings"]
    )

    # Generate script content
    sbatch_header = _generate_sbatch_header(process_config, slurm_job_workspace_config, use_gpu=True)
    script_content = _generate_slurm_script_content(
        process_config, slurm_job_workspace_config, sbatch_header,
        apptainer_command, af3_command, running_path_template,
        failed_dir_gcs, success_dir_gcs, result_dir_gcs, None # No post-success for inference
    )

    # Write script file
    script_path = os.path.join(slurm_job_workspace_config.script_dir, f"{data_pipeline_name}.sh")
    logger.debug(f"Writing inference SLURM script to {script_path}")
    try:
        with open(script_path, "w", encoding="utf-8") as f: f.write(script_content)
        os.chmod(script_path, 0o755) # Ensure executable
    except IOError as e:
        logger.error(f"Failed to write or chmod script file {script_path}: {e}")
        raise

    return script_path, running_path_template # Return template


def download_blob(
    storage_client: storage.Client,
    bucket_name: str,
    source_blob_name_rel: str, # Relative path
    destination_folder: str,
) -> Optional[str]:
    """Download blob from GCS to local folder."""
    gcs_path = f"gs://{bucket_name}/{source_blob_name_rel}"
    file_name = os.path.basename(source_blob_name_rel)
    destination_file_name = os.path.join(destination_folder, file_name)
    try:
        logger.debug(f"Downloading {gcs_path} to {destination_file_name}")
        bucket = storage_client.bucket(bucket_name)
        blob = bucket.blob(source_blob_name_rel)
        blob.download_to_filename(destination_file_name)
        logger.debug(f"Download complete.")
        return destination_file_name
    except Exception as e:
        logger.error(f"Download failed for {gcs_path}: {e}")
        return None # Let caller handle failure

def submit_job(script_path: str) -> Optional[str]:
    """Submit SLURM job via sbatch and return job ID."""
    try:
        result = run_command(["sbatch", script_path])
        if result is None: return None
        job_id = search_regex_pattern(r"Submitted batch job (\d+)", result)
        if job_id:
            logger.info(f"Submitted SLURM job from {script_path}, Job ID: {job_id}")
            return job_id
        else:
            logger.error(f"sbatch submission failed or no Job ID parsed from output for {script_path}. Output: {result}")
            return None
    except Exception as e:
         logger.error(f"Error submitting job {script_path}: {e}")
         return None

def _determine_fail_path(process_config: ProcessConfig, blobname_rel: str, timestamp: str) -> str:
     """Helper to determine the correct GCS fail path (full gs:// path)."""
     file_name = os.path.basename(blobname_rel)
     gcs_base = f"gs://{process_config.bucket_name}"

     if process_config.is_inference:
         # Heuristic based on input path e.g., inference_toprocess/PIPELINE_NAME/input.json
         pipeline_name_for_fail = os.path.basename(os.path.dirname(blobname_rel)) or os.path.splitext(file_name)[0]
         # Structure: <base_dir>/<input_name>/<timestamp>/
         fail_path_dir = f"{process_config.bucket_failed_dir}{pipeline_name_for_fail}/{timestamp}/"
     else:
         pipeline_name_for_fail = os.path.splitext(file_name)[0]
         # Structure: <base_dir>/<input_name>/<timestamp>/
         fail_path_dir = f"{process_config.bucket_failed_dir}{pipeline_name_for_fail}/{timestamp}/"

     # Construct full path including filename
     # Ensure fail_path_dir ends with a slash (already handled by f-string)
     fail_path_rel = os.path.join(fail_path_dir, file_name) # Use os.path.join for safety

     return f"{gcs_base}/{fail_path_rel}"

def submit_slurm_single_job(
    storage_client: storage.Client,
    process_config: ProcessConfig,
    timestamp: str,
    blobname_rel: str, # Relative path within bucket
    slurm_job_workspace_config: SlurmJobWorkspaceConfig,
) -> Tuple[bool, Optional[SlurmJobInfo]]:
    """Download input, generate script, submit job, move input blob."""
    local_input_file_path = None
    job_id = None
    gcs_source_blob_path_full = f"gs://{process_config.bucket_name}/{blobname_rel}"
    # Determine fail path *before* potential errors
    bucket_fail_path_gcs = _determine_fail_path(process_config, blobname_rel, timestamp)

    try:
        # 1. Download input file
        local_input_file_path = download_blob(
            storage_client=storage_client, bucket_name=process_config.bucket_name,
            source_blob_name_rel=blobname_rel,
            destination_folder=slurm_job_workspace_config.running_dir,
        )
        if not local_input_file_path: raise Exception(f"Failed to download {gcs_source_blob_path_full}")

        # 2. Generate SLURM script
        if not process_config.is_inference:
            script_path, running_path_template = generate_data_pipeline_slurm_script(
                process_config, slurm_job_workspace_config, timestamp, local_input_file_path)
        else:
            script_path, running_path_template = generate_inference_slurm_script(
                process_config, slurm_job_workspace_config, timestamp, local_input_file_path)

        # 3. Submit the job
        job_id = submit_job(script_path)
        if not job_id: raise Exception(f"SLURM job submission failed for script {script_path}")

        # 4. Move input blob in GCS to running path
        bucket_running_path_final = running_path_template.replace("$SLURM_JOB_ID", job_id)
        # Extract relative path for move function
        dest_rel_path = bucket_running_path_final.replace(f"gs://{process_config.bucket_name}/", "")
        move_bucket_file(storage_client, process_config.bucket_name, blobname_rel, dest_rel_path)

        logger.info(f"Successfully submitted job {job_id} for input {gcs_source_blob_path_full}.")
        slurm_log_base_dir = slurm_job_workspace_config.log_dir
        slurm_output_log = os.path.join(slurm_log_base_dir, f"job_{job_id}", "out.txt")
        slurm_error_log = os.path.join(slurm_log_base_dir, f"job_{job_id}", "err.txt")
        logger.info(f"  └── SLURM logs expected at: {slurm_output_log} (out) and {slurm_error_log} (err)")

        return True, SlurmJobInfo(
            job_id=job_id,
            bucket_running_file_path=bucket_running_path_final, # Store full gs:// path
            bucket_fail_file_path=bucket_fail_path_gcs,        # Store full gs:// path
        )

    except Exception as e:
        logger.error(f"Failed process/submit for {gcs_source_blob_path_full}: {e}")
        # Attempt to move original input blob to fail path if it wasn't already moved
        # Check if the source blob still exists in the original location
        try:
             source_blob = storage_client.bucket(process_config.bucket_name).blob(blobname_rel)
             if source_blob.exists():
                  logger.info(f"Attempting to move failed input {gcs_source_blob_path_full} to {bucket_fail_path_gcs}")
                  fail_rel_path = bucket_fail_path_gcs.replace(f"gs://{process_config.bucket_name}/", "")
                  move_bucket_file(storage_client, process_config.bucket_name, blobname_rel, fail_rel_path)
             else:
                  logger.warning(f"Source blob {gcs_source_blob_path_full} no longer exists, likely moved or deleted during failed process.")
        except Exception as move_e:
             logger.error(f"Failed to move blob {blobname_rel} to fail path after primary error: {move_e}")

        # Clean up local workspace / downloaded file
        if local_input_file_path and os.path.exists(local_input_file_path):
            try: os.remove(local_input_file_path); logger.debug(f"Removed local file {local_input_file_path}")
            except OSError as rm_e: logger.error(f"Error removing {local_input_file_path}: {rm_e}")
        if os.path.exists(slurm_job_workspace_config.base_dir):
             try: shutil.rmtree(slurm_job_workspace_config.base_dir); logger.debug(f"Removed local workspace {slurm_job_workspace_config.base_dir}")
             except Exception as rmtree_e: logger.error(f"Failed to remove {slurm_job_workspace_config.base_dir}: {rmtree_e}")

        return False, None


def check_job_timeout(
    storage_client: storage.Client,
    process_config: ProcessConfig,
    job_id: str,
    pending_job_time_limit: int,
    job_info: SlurmJobInfo
) -> bool:
    """Checks if a PENDING job has exceeded the time limit and cancels it."""
    current_time = datetime.datetime.now(datetime.timezone.utc)
    try:
        # Extract timestamp from the running file path (assumes format YYYYMMDD_HHMMSS_ffffff)
        # Example: gs://bucket/running_dir/YYYYMMDD_HHMMSS_ffffff/...
        blobname_split = job_info.bucket_running_file_path.split("/")
        if len(blobname_split) < 4: raise ValueError("Unexpected running file path format")
        timestamp_str = blobname_split[3] # Index 3 after gs:, bucket, running_dir
        job_timestamp = datetime.datetime.strptime(timestamp_str, "%Y%m%d_%H%M%S_%f").replace(tzinfo=datetime.timezone.utc)
    except (ValueError, IndexError, TypeError) as e:
        logger.error(f"Could not parse timestamp from {job_info.bucket_running_file_path} for job {job_id}: {e}")
        return False # Cannot determine age

    time_diff = (current_time - job_timestamp).total_seconds()
    if time_diff < pending_job_time_limit:
        # logger.debug(f"Pending job {job_id} age {time_diff:.0f}s < limit {pending_job_time_limit}s.")
        return False

    logger.warning(f"Pending job {job_id} exceeded time limit ({time_diff:.0f}s >= {pending_job_time_limit}s). Cancelling.")
    try:
        run_command(["scancel", job_id])
        logger.info(f"Cancelled pending job {job_id} via scancel.")
        # Move the GCS marker file to failed path
        source_rel_path = job_info.bucket_running_file_path.replace(f"gs://{process_config.bucket_name}/", "")
        fail_rel_path = job_info.bucket_fail_file_path.replace(f"gs://{process_config.bucket_name}/", "")
        move_bucket_file(storage_client, process_config.bucket_name, source_rel_path, fail_rel_path)
        logger.info(f"Moved timed-out pending job {job_id} marker to {job_info.bucket_fail_file_path}.")
        return True # Job was cancelled
    except Exception as e:
        logger.error(f"Error cancelling or moving timed-out pending job {job_id}: {e}")
        # Assume processed (attempted cancel) even on error here
        return True

def validate_job_state(
    storage_client: storage.Client,
    job_cache: Dict[str, SlurmJobInfo],
    process_config: ProcessConfig,
    pending_job_time_limit: Optional[int] = None,
) -> None:
    """Check status of tracked SLURM jobs, update cache/GCS, and cleanup GCS running dirs."""
    job_ids_to_remove = []
    if not job_cache: return # Skip if cache is empty

    logger.debug(f"Validating states for {len(job_cache)} tracked jobs...")
    for job_id, job_info in list(job_cache.items()): # Iterate copy
        job_state = get_job_state_scontrol(job_id=job_id)

        if job_state is None:
            logger.warning(f"Could not retrieve state for job {job_id}. Retrying next cycle.")
            continue # Keep in cache

        active_states = {"RUNNING", "CONFIGURING", "SUSPENDED", "REQUEUED", "COMPLETING"}
        terminal_failure_states = {
            "FAILED", "BOOT_FAIL", "CANCELLED", "DEADLINE", "TIMEOUT",
            "NODE_FAIL", "PREEMPTED", "OUT_OF_MEMORY"
        }

        processed_terminal = False # Flag to know if we should attempt cleanup

        if job_state in active_states:
            logger.debug(f"Job {job_id} is active ({job_state}).")
            continue
        elif job_state == "PENDING":
            if pending_job_time_limit is not None:
                 if check_job_timeout(storage_client, process_config, job_id, pending_job_time_limit, job_info):
                      # Timeout already moved marker and set state to CANCELLED (implicitly)
                      processed_terminal = True # Mark for cleanup after timeout handling
                      # No need to add to job_ids_to_remove here, timeout function handles it if successful
                 else:
                      logger.debug(f"Job {job_id} is PENDING (no timeout check).")
                      continue # Keep if pending and not timed out
            else:
                 logger.debug(f"Job {job_id} is PENDING (no timeout check).")
                 continue # Keep if pending
        elif job_state == "COMPLETED":
             logger.info(f"Job {job_id} COMPLETED.")
             # SLURM script should have moved the marker file already.
             processed_terminal = True
        elif job_state in terminal_failure_states or job_state == "UNKNOWN_OR_PURGED":
             state_reason = "failed/cancelled" if job_state != "UNKNOWN_OR_PURGED" else "unknown/purged"
             logger.warning(f"Job {job_id} is {state_reason} ({job_state}). Moving marker file if present.")
             try:
                 source_rel = job_info.bucket_running_file_path.replace(f"gs://{process_config.bucket_name}/", "")
                 fail_rel = job_info.bucket_fail_file_path.replace(f"gs://{process_config.bucket_name}/", "")
                 move_bucket_file(storage_client, process_config.bucket_name, source_rel, fail_rel)
                 logger.info(f"Moved marker file for {state_reason} job {job_id} to {job_info.bucket_fail_file_path}")
             except FileNotFoundError:
                  logger.warning(f"Running marker file not found for {state_reason} job {job_id} at {job_info.bucket_running_file_path}. Already moved or cleanup occurred?")
             except Exception as e:
                 logger.error(f"Failed to move marker for {state_reason} job {job_id}: {e}")
             # Mark for cleanup attempt even if move fails (the dir might still be there)
             processed_terminal = True
        else:
            logger.error(f"Job {job_id} has unrecognized SLURM state: {job_state}. Leaving in cache.")
            continue # Keep in cache for investigation


        # --- Unified Cleanup for Terminal States ---
        if processed_terminal:
            blob_prefix = None # Initialize in case of errors determining prefix
            running_marker_path = job_info.bucket_running_file_path # Full gs:// path

            try:
                # Attempt GCS cleanup using Python client library
                # Derive the prefix from the running marker path (remove gs://bucket_name/)
                gcs_base_uri = f"gs://{process_config.bucket_name}/"
                if running_marker_path.startswith(gcs_base_uri):
                    relative_marker_path = running_marker_path[len(gcs_base_uri):]
                    # The prefix is the 'directory' containing the marker file
                    # e.g., running_dir/JOB_NAME/TIMESTAMP/JOB_ID/
                    blob_prefix = os.path.dirname(relative_marker_path)
                    if blob_prefix and not blob_prefix.endswith('/'):
                        blob_prefix += '/' # Ensure prefix ends with / for directory matching

                    if blob_prefix:
                        logger.debug(f"Attempting GCS cleanup for job {job_id} using prefix: gs://{process_config.bucket_name}/{blob_prefix}")
                        bucket = storage_client.bucket(process_config.bucket_name)
                        blobs_iterator = bucket.list_blobs(prefix=blob_prefix)

                        deleted_count = 0
                        has_blobs = False
                        for blob in blobs_iterator:
                            has_blobs = True
                            try:
                                blob.delete()
                                deleted_count += 1
                            except Exception as delete_e:
                                 logger.error(f"Error deleting blob {blob.name} during cleanup for job {job_id}: {delete_e}")

                        # Log summary based on whether blobs were found/deleted
                        if not has_blobs:
                            logger.debug(f"Cleanup check complete for job {job_id}: No objects found under prefix '{blob_prefix}'.")
                        else:
                             logger.debug(f"GCS cleanup successful for job {job_id}: Deleted {deleted_count} object(s) under prefix '{blob_prefix}'.")
                    else:
                        logger.warning(f"Could not determine GCS prefix for job {job_id} from {running_marker_path}. Skipping GCS cleanup.")
                else:
                     logger.warning(f"Running marker path {running_marker_path} does not match expected bucket {gcs_base_uri}. Skipping GCS cleanup.")

            except Exception as cleanup_e:
                # This catches errors during bucket access, listing blobs, or other unexpected library issues
                path_context = f"gs://{process_config.bucket_name}/{blob_prefix}" if blob_prefix else running_marker_path # Use prefix if determined
                logger.error(f"Error during GCS cleanup attempt for job {job_id} (prefix context: {path_context}): {cleanup_e}")
                    
            # Add job to be removed from cache *after* attempting cleanup
            if job_id not in job_ids_to_remove: # Avoid duplicates if timeout handling already added it
                job_ids_to_remove.append(job_id)

    # Remove jobs outside the loop
    if job_ids_to_remove:
        logger.debug(f"Removing {len(job_ids_to_remove)} jobs from cache: {job_ids_to_remove}")
        for job_id in job_ids_to_remove:
            if job_id in job_cache: del job_cache[job_id]


def submit_slurm_jobs(
    storage_client: storage.Client,
    process_config: ProcessConfig,
    timestamp: str,
    job_cache: Dict[str, SlurmJobInfo],
) -> None:
    """List input files, setup workspace, and submit jobs."""
    input_dir_relative = process_config.bucket_input_dir
    logger.debug(f"Checking for new files in gs://{process_config.bucket_name}/{input_dir_relative}")
    has_submitted_any = False
    try:
        # Process blobs one by one to handle errors gracefully
        blob_iterator = listing_bucket_dir(storage_client, process_config.bucket_name, input_dir_relative)
        while True:
            try:
                blobname_relative = next(blob_iterator)
                if not blobname_relative.endswith(".json"): # Example filter
                     logger.debug(f"Skipping non-json item: {blobname_relative}")
                     continue

                logger.info(f"Found new input file: gs://{process_config.bucket_name}/{blobname_relative}")
                slurm_workspace = setup_slurm_job_workspace(process_config, timestamp, blobname_relative)
                is_success, job_info = submit_slurm_single_job(
                    storage_client, process_config, timestamp, blobname_relative, slurm_workspace
                )
                if is_success and job_info:
                     job_cache[job_info.job_id] = job_info
                     has_submitted_any = True
                # Error logging/handling/moving to fail is done within submit_slurm_single_job

            except StopIteration:
                break # No more blobs found
            except Exception as e:
                 # Log error for this specific blob and continue to the next
                 logger.error(f"Error processing blob during listing/submission in {input_dir_relative}: {e}")
                 # Avoid infinite loop if listing itself fails continuously
                 time.sleep(1) # Small delay before potentially retrying list

    except Exception as e:
        # Error during the initial listing call itself
        logger.error(f"Failed to list blobs in gs://{process_config.bucket_name}/{input_dir_relative}: {e}")

    if not has_submitted_any:
        logger.debug(f"No new valid files found to submit in {input_dir_relative} this cycle.")


def _deep_update(source, overrides):
    """Recursively update dict `source` with `overrides`, skipping None values in overrides."""
    for key, value in overrides.items():
        if isinstance(value, collections.abc.Mapping) and key in source and isinstance(source[key], dict):
            source[key] = _deep_update(source.get(key, {}), value)
        elif value is not None: # Only update if override value is explicitly provided (not None)
             source[key] = value
    return source

def load_process_config(
    args: argparse.Namespace,
) -> Tuple[Optional[ServiceConfig], Optional[ProcessConfig], Optional[ProcessConfig]]:
    """Load and merge configurations from defaults, file, and command line."""
    # Base defaults (ensure directories have trailing slashes)
    default_config = {
        "bucket_name": None, "time_interval": 30, "run_data_pipeline": True, "run_inference": True,
        "pending_job_time_limit": None, "sif_dir": "/opt/apps/af3/containers/",
        "db_dir": "/dev/shm/public_databases/", "model_dir": f"{HOME_DIR}/models/",
        "pdb_database_path": None, "jax_compilation_cache_path": None,
        "data_pipeline": {
            "bucket_input_dir": "data_pipeline_toprocess/", "bucket_running_dir": "data_pipeline_running/",
            "bucket_done_dir": "data_pipeline_success/", "bucket_failed_dir": "data_pipeline_failed/",
            "bucket_result_dir": "data_pipeline_results/", "local_work_dir": f"{HOME_DIR}/af3_data_pipeline",
            "partition_name": "data", "job_memory_size": 60, "job_cpu_count": 8, "job_timeout": 3600,
        },
        "inference": {
            "bucket_input_dir": "inference_toprocess/", "bucket_running_dir": "inference_running/",
            "bucket_done_dir": "inference_success/", "bucket_failed_dir": "inference_failed/",
            "bucket_result_dir": "inference_results/", "local_work_dir": f"{HOME_DIR}/af3_inference",
            "partition_name": "inference", "job_memory_size": 64, "job_cpu_count": 12, "job_timeout": 7200,
            "max_template_date": None, "conformer_max_iterations": None, "num_recycles": None,
            "num_diffusion_samples": None, "num_seeds": None, "save_embeddings": None,
        },
    }

    # 1. Load from config file
    config_from_file = {}
    if args.config_file:
        try: config_from_file = load_config(args.config_file)
        except Exception: return None, None, None # Fatal error already logged

    # 2. Merge defaults and file config
    merged_config = default_config.copy()
    merged_config = _deep_update(merged_config, config_from_file)

    # 3. Create dict of CLI args that were actually provided (not None)
    args_dict = vars(args)
    cli_provided_args = {k: v for k, v in args_dict.items() if v is not None}

    # 4. Map CLI args to nested config structure for override merge
    cli_nested_overrides = {}
    for cli_key, value in cli_provided_args.items():
         is_mapped = False
         # Check top-level keys first
         if cli_key in merged_config and not isinstance(merged_config[cli_key], dict):
             cli_nested_overrides[cli_key] = value; is_mapped = True
         # Check process-specific keys (e.g., data_pipeline_job_timeout -> data_pipeline: { job_timeout: ... })
         if not is_mapped:
             for prefix in ["data_pipeline", "inference"]:
                 prefix_ = prefix.replace('-', '_') + "_"
                 if cli_key.startswith(prefix_):
                     conf_key = cli_key[len(prefix_):]
                     if prefix not in cli_nested_overrides: cli_nested_overrides[prefix] = {}
                     cli_nested_overrides[prefix][conf_key] = value; is_mapped = True; break
         # Check inference specific keys (handled by above pattern)
         # Check run flags
         if not is_mapped and cli_key.startswith("run_"):
             cli_nested_overrides[cli_key] = value; is_mapped = True

         if not is_mapped and cli_key not in ['config_file']: # Ignore unmapped args except config_file
             logger.warning(f"Command line argument '{cli_key}' not recognized or mapped to config structure.")


    # 5. Merge CLI overrides into the config (CLI takes final precedence)
    final_config = _deep_update(merged_config, cli_nested_overrides)

    # --- Create Dataclass Instances ---
    try:
        bucket_name = final_config.get("bucket_name")
        if not bucket_name: raise ValueError("Bucket name is required (use --bucket-name or set in config).")

        service_config = ServiceConfig(
            time_interval=final_config["time_interval"],
            run_data_pipeline=final_config["run_data_pipeline"],
            run_inference=final_config["run_inference"],
            pending_job_time_limit=final_config.get("pending_job_time_limit")
        )

        def create_process_config(prefix: str, is_inference: bool) -> ProcessConfig:
            proc_conf_dict = final_config[prefix] # Get the specific process config dict
            # Add global/shared settings
            proc_conf_dict.update({
                "bucket_name": bucket_name, "sif_dir": final_config["sif_dir"],
                "db_dir": final_config["db_dir"], "model_dir": final_config["model_dir"],
                "is_inference": is_inference,
                "pdb_database_path": final_config.get("pdb_database_path") if not is_inference else None,
                "jax_compilation_cache_path": final_config.get("jax_compilation_cache_path") if is_inference else None
            })
            # Add conditional bucket_submit_inference_dir for data pipeline
            if not is_inference and final_config.get("run_inference", False): # Check run_inference flag
                 proc_conf_dict["bucket_submit_inference_dir"] = final_config["inference"]["bucket_input_dir"]

            logger.debug(f"Preprocessing config dict for {prefix} before dataclass creation (initial): {proc_conf_dict}")
            for key, value in list(proc_conf_dict.items()): # Iterate over a copy of items for safe modification
                if value == '':
                    logger.debug(f"Converting empty string value for key '{key}' to None in {prefix} config.")
                    proc_conf_dict[key] = None
            logger.debug(f"Preprocessing config dict for {prefix} before dataclass creation (final): {proc_conf_dict}")

            # Filter only valid fields for ProcessConfig dataclass
            valid_keys = {f.name for f in dataclasses.fields(ProcessConfig)}
            filtered_dict = {k: v for k, v in proc_conf_dict.items() if k in valid_keys}
            # Validate required fields are present after filtering (redundant if defaults are good)
            for field in dataclasses.fields(ProcessConfig):
                if field.default == dataclasses.MISSING and field.default_factory == dataclasses.MISSING:
                     if field.name not in filtered_dict:
                          raise ValueError(f"Missing required config value for '{prefix}.{field.name}'")
            return ProcessConfig(**filtered_dict)

        # Only create config if the corresponding run flag is True
        data_pipeline_config = create_process_config("data_pipeline", False) if final_config.get("run_data_pipeline") else None
        inference_config = create_process_config("inference", True) if final_config.get("run_inference") else None

        logger.info(f"Final Service Config: {service_config}")
        logger.info(f"Data Pipeline Active: {bool(data_pipeline_config)}. Config: {data_pipeline_config if data_pipeline_config else 'N/A'}")
        logger.info(f"Inference Active: {bool(inference_config)}. Config: {inference_config if inference_config else 'N/A'}")

        # Basic validation: ensure input dirs end with '/'
        def ensure_trailing_slash(cfg, attr_name):
            value = getattr(cfg, attr_name, None)
            if value and isinstance(value, str) and not value.endswith('/'):
                setattr(cfg, attr_name, value + '/')

        dir_attributes_to_check = [
            "bucket_input_dir", "bucket_running_dir", "bucket_done_dir",
            "bucket_failed_dir", "bucket_result_dir", "bucket_submit_inference_dir"
        ]

        for cfg in [data_pipeline_config, inference_config]:
            if cfg:
                for attr in dir_attributes_to_check:
                    ensure_trailing_slash(cfg, attr)

        return service_config, data_pipeline_config, inference_config

    except (KeyError, ValueError, TypeError) as e:
        logger.critical(f"Configuration error during final processing: {e}. Check config file/args.")
        logger.debug(f"Final config state before error: {final_config}")
        return None, None, None

# --- Main Loop ---

def main(args: argparse.Namespace) -> None:
    service_config, data_pipeline_config, inference_config = load_process_config(args)
    if not service_config: exit(1) # Exit if config loading failed

    job_cache_map: Dict[str, Dict[str, SlurmJobInfo]] = {"data_pipeline": {}, "inference": {}}
    storage_client = storage.Client()

    logger.info(f"AF3 service started. Log directory: {LOG_DIR}")
    # Log watching status based on actual loaded config
    if data_pipeline_config: logger.info(f"Watching Data Pipeline: gs://{data_pipeline_config.bucket_name}/{data_pipeline_config.bucket_input_dir}")
    else: logger.info("Data Pipeline processing is disabled.")
    if inference_config: logger.info(f"Watching Inference: gs://{inference_config.bucket_name}/{inference_config.bucket_input_dir}")
    else: logger.info("Inference processing is disabled.")
    if service_config.pending_job_time_limit: logger.info(f"Pending job time limit: {service_config.pending_job_time_limit}s.")

    while True:
        current_time = datetime.datetime.now(datetime.timezone.utc) # Use UTC time
        timestamp = current_time.strftime("%Y%m%d_%H%M%S_%f") # Microsecond precision
        logger.debug(f"--- Cycle Start: {current_time.isoformat()} ---")

        # --- Validate States (only if process is enabled) ---
        if data_pipeline_config:
             validate_job_state(storage_client, job_cache_map["data_pipeline"], data_pipeline_config, service_config.pending_job_time_limit)
        if inference_config:
             validate_job_state(storage_client, job_cache_map["inference"], inference_config, service_config.pending_job_time_limit)

        # --- Submit New (only if process is enabled) ---
        if data_pipeline_config:
             submit_slurm_jobs(storage_client, data_pipeline_config, timestamp, job_cache_map["data_pipeline"])
        if inference_config:
             submit_slurm_jobs(storage_client, inference_config, timestamp, job_cache_map["inference"])

        dp_cache = len(job_cache_map["data_pipeline"])
        inf_cache = len(job_cache_map["inference"])
        logger.debug(f"Active jobs cache: DataPipeline={dp_cache}, Inference={inf_cache}")
        logger.debug(f"--- Cycle End. Waiting {service_config.time_interval}s ---")
        time.sleep(service_config.time_interval)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Watches GCS buckets and submits AlphaFold 3 jobs to SLURM."
    )
    args = parse_arguments(parser)
    main(args)
