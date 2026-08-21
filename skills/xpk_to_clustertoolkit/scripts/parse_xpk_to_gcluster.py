#!/usr/bin/env python3
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

"""Parser utility for translating XPK CLI commands to Cluster Toolkit configurations."""

import argparse
import json
import os
import re
import shlex
import sys
from typing import Any

import yaml


def dump_yaml(data: dict[str, Any]) -> str:
  """Serializes data dictionary into YAML format preserving key insertion order."""
  return yaml.dump(data, default_flow_style=False, sort_keys=False)


TPU_CHIPS_TO_TOPOLOGY = {
    # 2D TPUs (v6e, v5e, v5litepod)
    "2d": {
        1: "1x1",
        4: "2x2",
        8: "2x4",
        16: "4x4",
        32: "4x8",
        64: "8x8",
        128: "8x16",
        256: "16x16",
        512: "16x32",
        1024: "32x32",
    },
    # 3D TPUs (tpu7x, v4, v5p)
    "3d": {
        8: "2x2x2",
        16: "2x2x4",
        32: "2x4x4",
        64: "4x4x4",
        128: "4x4x8",
        256: "4x8x8",
        512: "8x8x8",
        1024: "8x8x16",
        2048: "8x16x16",
        4096: "16x16x16",
    },
}

TPU_FAMILY_CONFIG = {
    "v6e": ("ct6e-standard-4t", "2d"),
    "v5e": ("ct5lp-hightpu-4t", "2d"),
    "v5litepod": ("ct5lp-hightpu-4t", "2d"),
    "v5p": ("ct5p-hightpu-4t", "3d"),
    "v4": ("ct4p-hightpu-4t", "3d"),
    "tpu7x": ("tpu7x-standard-4t", "3d"),
}


def get_machine_type(device_type: str) -> tuple[str, str]:
  """Resolves GCE machine type and TPU topology from XPK device type."""
  # 1. Check relative path to canonical pkg/config/machine_mappings.json
  ref_path = os.path.normpath(
      os.path.join(
          os.path.dirname(__file__),
          "..",
          "..",
          "..",
          "pkg",
          "config",
          "machine_mappings.json",
      )
  )

  shorthand_map: dict[str, str] = {}
  if os.path.exists(ref_path):
    try:
      with open(ref_path, "r", encoding="utf-8") as f:
        data = json.load(f)
        shorthand_map = data.get("accelerator_shorthand_map", {})
    except Exception:
      pass

  # 2. Check direct match in accelerator shorthand map (GPU & fixed TPUs)
  if device_type in shorthand_map:
    mapped = shorthand_map[device_type]
    if not any(device_type.startswith(p) for p in ("v4", "v5", "v6", "tpu7x")):
      return mapped, "N/A"

  # 3. Dynamic TPU Topology & Machine Type Resolution
  # Case A: Explicit dimension topology in device_type (e.g., tpu7x-4x4x4, v6e-4x4, v4-2x2x4)
  top_match = re.match(
      r"^(tpu7x|v6e|v5p|v5e|v5litepod|v4)-(\d+x\d+(?:x\d+)?)$",
      device_type,
      re.IGNORECASE,
  )
  if top_match:
    family, topology = top_match.group(1).lower(), top_match.group(2)
    default_machine_type, _ = TPU_FAMILY_CONFIG.get(
        family, (shorthand_map.get(family, f"{family}-standard-4t"), "3d")
    )
    machine_type = shorthand_map.get(device_type, default_machine_type)
    return machine_type, topology

  # Case B: Chip count in device_type (e.g., v6e-1, v6e-16, tpu7x-128, v4-128, v5p-64)
  chip_match = re.match(
      r"^(tpu7x|v6e|v5p|v5e|v5litepod|v4)-(\d+)$",
      device_type,
      re.IGNORECASE,
  )
  if chip_match:
    family, chips_str = chip_match.group(1).lower(), chip_match.group(2)
    chips = int(chips_str)
    if family in TPU_FAMILY_CONFIG:
      default_machine_type, dim_type = TPU_FAMILY_CONFIG[family]
      machine_type = shorthand_map.get(device_type, default_machine_type)
      topology = TPU_CHIPS_TO_TOPOLOGY[dim_type].get(chips, "N/A")
      return machine_type, topology

  # 4. Fallback to direct mapping from shorthand_map if available
  if device_type in shorthand_map:
    return shorthand_map[device_type], "N/A"

  return device_type, "N/A"


def is_tpu_hardware(compute_type: str | None, device_type: str | None) -> bool:
  """Returns True if the compute hardware matches TPU prefix patterns."""
  c_lower = (compute_type or "").lower()
  d_lower = (device_type or "").lower()
  if "tpu" in c_lower or "tpu" in d_lower:
    return True
  pattern = r"^(v[2-9]|ct[2-9]|tpu)"
  return bool(re.search(pattern, c_lower) or re.search(pattern, d_lower))


def is_flag_true(raw_val: Any) -> bool:
  """Parses flexible boolean argument representations safely."""
  if raw_val is None:
    return False
  val_lower = str(raw_val).lower()
  return val_lower in ("true", "1", "yes")


def parse_workload_create(
    is_pathways: bool, unknown: list[str]
) -> tuple[list[str], list[str], list[str]]:
  """Parses XPK workload create arguments into gcluster job submit command tokens."""
  warnings: list[str] = []
  cmd = ["gcluster", "job", "submit"]
  if is_pathways:
    cmd.append("--pathways")

  value_flags = {
      "--workload": "--name",
      "--cluster": "--cluster",
      "--project": "--project",
      "--zone": "--location",
      "--num-slices": "--num-slices",
      "--num-nodes": "--num-nodes",
      "--docker-image": "--image",
      "--command": "--command",
      "--priority": "--priority",
      "--max-restarts": "--restarts",
      "--base-docker-image": "--base-image",
      "--script-dir": "--build-context",
      "--ttl-seconds-after-finished": "--gke-ttl-after-finished",
      "--termination-grace-period-seconds": "--grace-period",
      "--scheduler": "--gke-scheduler",
      "--ramdisk-directory": "--gke-mtc-ramdisk-dir",
      "--restart-on-exit-codes": "--restart-on-exit-codes",
      "--service-account": "--service-account",
      "--service-account-name": "--service-account",
      "--image-pull-secret": "--image-pull-secret",
      "--placement-policy": "--placement-policy",
      "--node-constraint": "--node-constraint",
      "--output-manifest-file": "--dry-run-out",
      "--timeout": "--timeout",
      "--queue": "--queue",
      "--gke-namespace": "--gke-namespace",
      # Pathways specific value flags
      "--proxy-server-image": "--pathways-proxy-server-image",
      "--server-image": "--pathways-server-image",
      "--pathways-gcs-location": "--pathways-gcs-location",
      "--custom-pathways-server-args": "--pathways-server-args",
      "--custom-pathways-proxy-server-args": "--pathways-proxy-args",
      "--custom-pathways-worker-args": "--pathways-worker-args",
      "--elastic-slices": "--pathways-elastic-slices",
      "--max-slice-restarts": "--pathways-max-slice-restarts",
  }

  boolean_flags = {
      "--wait-for-job-completion": "--await-job-completion",
      "--mtc-enabled": "--gke-mtc-enabled",
      "--headless": "--pathways-headless",
      "--enable-debug-logs": "--verbose",
      "--skip-prereqs": "--skip-prereqs",
  }

  parser = argparse.ArgumentParser(allow_abbrev=False)
  for flag in value_flags.keys():
    parser.add_argument(flag, type=str, nargs="?")
  for flag in boolean_flags.keys():
    parser.add_argument(flag, nargs="?", const="true")

  parser.add_argument("--tpu-type", type=str)
  parser.add_argument("--device-type", type=str)
  parser.add_argument("--use-parallel-containers", type=str, nargs="?")
  parser.add_argument("--env", action="append", default=[])
  parser.add_argument("--storage", action="append", default=[])
  parser.add_argument("--pathways-proxy-env", action="append", default=[])
  parser.add_argument("--pathways-server-env", action="append", default=[])
  parser.add_argument("--pathways-worker-env", action="append", default=[])

  parsed, rest = parser.parse_known_args(unknown)

  device_type = parsed.tpu_type or parsed.device_type
  m_type, top = (None, None)
  if device_type:
    m_type, top = get_machine_type(device_type)

  is_tpu = bool(parsed.tpu_type) or is_tpu_hardware(m_type, device_type)

  for flag, mapped_flag in value_flags.items():
    val = getattr(parsed, flag.lstrip("-").replace("-", "_"), None)
    if val is not None:
      if mapped_flag == "--num-nodes" and is_tpu:
        warnings.append(
            "Omitted --num-nodes because it is not supported for TPU jobs in"
            " gcluster."
        )
        continue
      cmd.extend([mapped_flag, val])

  for flag, mapped_flag in boolean_flags.items():
    raw_val = getattr(parsed, flag.lstrip("-").replace("-", "_"), None)
    if is_flag_true(raw_val):
      cmd.append(mapped_flag)

  if device_type:
    cmd.extend(["--compute-type", m_type])
    if top and top != "N/A":
      cmd.extend(["--topology", top])
    elif is_tpu and (device_type.startswith("<") or device_type.startswith("$")):
      cmd.extend(["--topology", "<YOUR_TOPOLOGY>"])

  if (
      parsed.use_parallel_containers is not None
      and str(parsed.use_parallel_containers).lower() == "false"
  ):
    cmd.append("--gke-disable-parallel-containers")

  for env_val in parsed.env:
    cmd.extend(["--env", env_val])
  for storage_val in parsed.storage:
    if ";" in storage_val:
      cmd.extend(["--mount", storage_val])
    else:
      last_component = storage_val.rstrip("/").split("/")[-1]
      dest_name = re.sub(r"[\$\{\}\<\>]", "", last_component).strip().lower()
      clean_dest = dest_name if dest_name else "storage"
      cmd.extend(["--mount", f"{storage_val};/mnt/{clean_dest};rw"])
  for env_val in parsed.pathways_proxy_env:
    cmd.extend(["--pathways-proxy-env", env_val])
  for env_val in parsed.pathways_server_env:
    cmd.extend(["--pathways-server-env", env_val])
  for env_val in parsed.pathways_worker_env:
    cmd.extend(["--pathways-worker-env", env_val])

  return cmd, warnings, rest


def parse_cluster_create(
    is_pathways: bool, unknown: list[str]
) -> tuple[dict[str, Any], list[str], list[str]]:
  """Parses XPK cluster create arguments into Cluster Toolkit blueprint data structure."""
  parser = argparse.ArgumentParser(allow_abbrev=False)
  parser.add_argument("--cluster", type=str)
  parser.add_argument("--project", type=str)
  parser.add_argument("--zone", type=str)
  parser.add_argument("--num-slices", type=str)
  parser.add_argument("--tpu-type", type=str)
  parser.add_argument("--device-type", type=str)
  parser.add_argument("--default-pool-cpu-machine-type", type=str)
  parser.add_argument("--cluster-cpu-machine-type", type=str)
  parser.add_argument("--reservation", type=str)
  parser.add_argument("--default-pool-cpu-num-nodes", type=str)
  parser.add_argument("--gke-version", type=str)
  parser.add_argument("--authorized-networks", type=str)
  parser.add_argument("--private-endpoint-subnetwork", type=str)
  parser.add_argument("--num-nodes", type=str)
  parser.add_argument("--autoprovisioning-min-chips", type=str)
  parser.add_argument("--autoprovisioning-max-chips", type=str)
  parser.add_argument("--host-maintenance-interval", type=str)
  parser.add_argument("--mtc-ramdisk-size", type=str)
  parser.add_argument("--mtc-gcs-bucket", type=str)
  parser.add_argument("--mtc-toleration-key", type=str)
  parser.add_argument("--tensorboard-region", type=str)
  parser.add_argument("--tensorboard-name", type=str)

  boolean_flags = [
      "--on-demand",
      "--spot",
      "--private",
      "--enable-private-endpoint",
      "--enable-master-global-access",
      "--enable-workload-identity",
      "--enable-gcsfuse-csi-driver",
      "--enable-lustre-csi-driver",
      "--enable-gcpfilestore-csi-driver",
      "--enable-parallelstore-csi-driver",
      "--enable-pd-csi-driver",
      "--flex",
      "--enable-autoprovisioning",
      "--enable-pathways",
      "--enable-mtc",
      "--create-vertex-tensorboard",
      "--managed-mldiagnostics",
  ]
  for flag in boolean_flags:
    parser.add_argument(flag, nargs="?", const="true")

  parsed, rest = parser.parse_known_args(unknown)

  deployment_name = parsed.cluster or "my-cluster"
  blueprint: dict[str, Any] = {
      "blueprint_name": deployment_name,
      "vars": {"deployment_name": deployment_name},
  }

  v = blueprint["vars"]
  v["project_id"] = parsed.project or "<YOUR_PROJECT_ID>"
  if parsed.zone and parsed.zone.startswith("$"):
    v["zone"] = parsed.zone
    if parsed.zone.startswith("${") and parsed.zone.endswith("}"):
      v["region"] = f"${{{parsed.zone[2:-1]}%-*}}"
    else:
      v["region"] = f"${{{parsed.zone[1:]}%-*}}"
  elif parsed.zone and "-" in parsed.zone:
    v["zone"] = parsed.zone
    v["region"] = parsed.zone.rsplit("-", 1)[0]
  else:
    v["zone"] = parsed.zone or "<YOUR_ZONE>"
    v["region"] = "<YOUR_REGION>"
  if parsed.num_slices:
    v["num_slices"] = parsed.num_slices

  device_type = parsed.tpu_type or parsed.device_type
  if device_type:
    m_type, top = get_machine_type(device_type)
    v["machine_type"] = m_type
    is_tpu = bool(parsed.tpu_type) or is_tpu_hardware(m_type, device_type)
    if top and top != "N/A":
      v["tpu_topology"] = top
    elif is_tpu and (device_type.startswith("<") or device_type.startswith("$")):
      v["tpu_topology"] = "<YOUR_TOPOLOGY>"

  warnings: list[str] = []
  if is_pathways or is_flag_true(parsed.enable_pathways):
    v["enable_pathways_for_tpus"] = True
    warnings.append(
        "enable_pathways_for_tpus: true automatically provisions the dedicated"
        " cpu-np CPU node pool with default n4-standard-64 instances."
    )

  cpu_machine_type = (
      parsed.default_pool_cpu_machine_type or parsed.cluster_cpu_machine_type
  )
  if cpu_machine_type:
    v["system_node_pool_machine_type"] = cpu_machine_type
  if parsed.reservation:
    v["reservation"] = parsed.reservation
  if is_flag_true(parsed.spot):
    v["spot"] = True
  if parsed.default_pool_cpu_num_nodes:
    v["system_pool_node_count"] = parsed.default_pool_cpu_num_nodes
  if parsed.gke_version:
    v["gke_version"] = parsed.gke_version
  if is_flag_true(parsed.private) or is_flag_true(
      parsed.enable_private_endpoint
  ):
    v["enable_private_endpoint"] = True
  if parsed.authorized_networks:
    v["authorized_cidr"] = parsed.authorized_networks
  if parsed.private_endpoint_subnetwork:
    v["private_endpoint_subnetwork"] = parsed.private_endpoint_subnetwork
  if is_flag_true(parsed.enable_master_global_access):
    v["enable_master_global_access"] = True
  if is_flag_true(parsed.enable_workload_identity):
    v["enable_workload_identity"] = True
  if is_flag_true(parsed.enable_gcsfuse_csi_driver):
    v["enable_gcsfuse_csi_driver"] = True
  if is_flag_true(parsed.enable_lustre_csi_driver):
    v["enable_managed_lustre_csi"] = True
  if is_flag_true(parsed.enable_parallelstore_csi_driver):
    v["enable_parallelstore_csi_driver"] = True
  if is_flag_true(parsed.enable_gcpfilestore_csi_driver):
    v["enable_filestore_csi_driver"] = True
  if is_flag_true(parsed.enable_pd_csi_driver):
    v["enable_pd_csi"] = True
  if parsed.num_nodes:
    v["static_node_count"] = parsed.num_nodes
  if is_flag_true(parsed.flex):
    v["enable_flex_start"] = True
  if is_flag_true(parsed.enable_autoprovisioning):
    v["enable_autoprovisioning"] = True
  if parsed.autoprovisioning_min_chips:
    v["autoprovisioning_min_chips"] = parsed.autoprovisioning_min_chips
  if parsed.autoprovisioning_max_chips:
    v["autoprovisioning_max_chips"] = parsed.autoprovisioning_max_chips
  if parsed.host_maintenance_interval:
    v["maintenance_interval"] = parsed.host_maintenance_interval
  if is_flag_true(parsed.enable_mtc):
    v["enable_mtc"] = True
  if parsed.mtc_ramdisk_size:
    v["mtc_ramdisk_size"] = parsed.mtc_ramdisk_size
  if parsed.mtc_gcs_bucket:
    v["mtc_gcs_bucket"] = parsed.mtc_gcs_bucket
  if parsed.mtc_toleration_key:
    v["mtc_toleration_key"] = parsed.mtc_toleration_key
  if is_flag_true(parsed.managed_mldiagnostics):
    v["enable_managed_mldiagnostics"] = True

  if (
      is_flag_true(parsed.create_vertex_tensorboard)
      or parsed.tensorboard_name
      or parsed.tensorboard_region
  ):
    warnings.append(
        "Cluster Toolkit does not support Vertex Tensorboard creation"
        " (--create-vertex-tensorboard/--tensorboard-name/--tensorboard-region)."
        " Please configure Tensorboard manually if needed."
    )

  return blueprint, warnings, rest


def format_cluster_output(
    blueprint: dict[str, Any], warnings: list[str], unmapped_flags: list[str]
) -> str:
  """Formats cluster creation blueprint and commands into printable string."""
  result = ""
  for w in warnings:
    result += f"# Note: {w}\n"
  if unmapped_flags:
    result += (
        "# Warning: Unmapped xpk flags were ignored:"
        f" {' '.join(unmapped_flags)}\n"
    )

  # Extract deployment-specific vars for the CLI command
  v = blueprint.get("vars", {})
  project_id = v.get("project_id") or "<YOUR_PROJECT_ID>"
  zone = v.get("zone") or "<YOUR_ZONE>"
  region = v.get("region") or "<YOUR_REGION>"
  deployment_name = v.get("deployment_name") or "my-cluster"

  # Clear them in the blueprint to keep it generic and reusable
  v["project_id"] = None
  v["zone"] = None
  v["region"] = None
  v["deployment_name"] = None

  result += "Parsed Blueprint Configuration:\n"
  result += dump_yaml(blueprint)

  vars_list = [
      f"project_id={project_id}",
      f"deployment_name={deployment_name}",
      f"zone={zone}",
      f"region={region}",
  ]
  vars_str = ",".join(vars_list)

  result += "\nCommands to run:\n"
  result += f"gcluster deploy {deployment_name}.yaml --vars {vars_str}\n"
  return result


def format_workload_output(
    cmd_tokens: list[str], warnings: list[str], unmapped_flags: list[str]
) -> str:
  """Formats workload creation command tokens into printable string."""
  result = ""
  for w in warnings:
    result += f"# Note: {w}\n"
  if unmapped_flags:
    result += (
        "# Warning: Unmapped xpk flags were ignored:"
        f" {' '.join(unmapped_flags)}\n"
    )
  result += shlex.join(cmd_tokens)
  return result


def parse_xpk_command(cmd_string: str) -> str:
  """Parses XPK command string and returns Cluster Toolkit output."""
  try:
    tokens = shlex.split(cmd_string)
  except ValueError as e:
    return f"Error parsing command: {e}"

  try:
    xpk_idx = tokens.index("xpk")
  except ValueError:
    return "Error: Not an xpk command"

  tokens = tokens[xpk_idx:]

  if len(tokens) < 3:
    return "Error: Incomplete xpk command"

  subcommand = f"{tokens[1]} {tokens[2]}"

  if subcommand == "workload create":
    cmd_tokens, warnings, unmapped = parse_workload_create(
        is_pathways=False, unknown=tokens[3:]
    )
    return format_workload_output(cmd_tokens, warnings, unmapped)
  elif subcommand == "workload create-pathways":
    cmd_tokens, warnings, unmapped = parse_workload_create(
        is_pathways=True, unknown=tokens[3:]
    )
    return format_workload_output(cmd_tokens, warnings, unmapped)
  elif subcommand in ["cluster create", "cluster create-pathways"]:
    is_pathways = subcommand == "cluster create-pathways"
    blueprint, warnings, unmapped = parse_cluster_create(
        is_pathways=is_pathways, unknown=tokens[3:]
    )
    return format_cluster_output(blueprint, warnings, unmapped)
  else:
    return (
        "Error: This script is for complex commands ('workload create',"
        " 'workload create-pathways', 'cluster create', 'cluster"
        f" create-pathways'). For '{subcommand}', please map flags natively"
        " using your reasoning."
    )


def main(argv: list[str] | None = None) -> None:
  if argv is None:
    argv = sys.argv

  args = argv[1:]
  if not args:
    print(
        'Usage: parse_xpk_to_gcluster.py [--xpk_command="..."]'
        " <xpk_command_string>"
    )
    sys.exit(1)

  cmd_string = None
  if args[0].startswith("--xpk_command="):
    cmd_string = args[0].split("=", 1)[1]
  elif args[0] == "--xpk_command" and len(args) == 2:
    cmd_string = args[1]
  elif args[0] == "--xpk_command" and len(args) > 2:
    cmd_string = shlex.join(args[1:])
  elif len(args) == 1:
    cmd_string = args[0]
  else:
    cmd_string = shlex.join(args)

  if not cmd_string:
    print(
        'Usage: parse_xpk_to_gcluster.py [--xpk_command="..."]'
        " <xpk_command_string>"
    )
    sys.exit(1)

  print(parse_xpk_command(cmd_string))


if __name__ == "__main__":
  main(sys.argv)
