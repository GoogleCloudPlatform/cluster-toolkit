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
import math
import re
import shlex
import sys
from typing import Any

try:
  import yaml
except ImportError:
  yaml = None


def dump_yaml(data: dict[str, Any]) -> str:
  """Serializes data dictionary into YAML format preserving key insertion order."""
  if yaml is not None:
    return yaml.dump(data, default_flow_style=False, sort_keys=False)

  def _emit(val: Any, indent: int = 0) -> list[str]:
    sp = " " * indent
    out = []
    if isinstance(val, dict):
      for k, v in val.items():
        if isinstance(v, (dict, list)):
          out.append(f"{sp}{k}:")
          out.extend(_emit(v, indent + 2))
        elif v is None:
          out.append(f"{sp}{k}: null")
        elif isinstance(v, bool):
          out.append(f"{sp}{k}: {str(v).lower()}")
        elif isinstance(v, (int, float)):
          out.append(f"{sp}{k}: {v}")
        else:
          out.append(f"{sp}{k}: {v}")
    elif isinstance(val, list):
      for item in val:
        if isinstance(item, dict):
          first = True
          for sub_k, sub_v in item.items():
            sub_sp = " " * (indent + 2)
            formatted_val = (
                "null"
                if sub_v is None
                else (str(sub_v).lower() if isinstance(sub_v, bool) else str(sub_v))
            )
            if first:
              out.append(f"{sp}- {sub_k}: {formatted_val}")
              first = False
            else:
              out.append(f"{sub_sp}{sub_k}: {formatted_val}")
        else:
          out.append(f"{sp}- {item}")
    return out

  return "\n".join(_emit(data)) + "\n"


NO_RESERVATION_AFFINITY: dict[str, Any] = {
    "consume_reservation_type": "NO_RESERVATION",
    "specific_reservations": [],
}

KNOWN_GPU_SUBSTRINGS: tuple[str, ...] = (
    "h100",
    "a100",
    "a2-",
    "a3-",
    "a4-",
    "a4x-",
    "l4-",
    "rtx-",
    "b200",
    "gb200",
    "v100",
    "t4",
    "p100",
)

GCE_MACHINE_TYPE_RE: re.Pattern[str] = re.compile(
    r"^[a-z][0-9a-z]*-(standard|highmem|highcpu|ultragpu|megagpu|highgpu|custom)-\d+"
)


def _tpu_machine_type(family: str, chips: int) -> str:
  """Maps TPU family and chip count to standard GCE machine type."""
  if family == "v6e":
    return "ct6e-standard-1t" if chips == 1 else (
        "ct6e-standard-8t" if chips == 8 else "ct6e-standard-4t"
    )
  if family in ("v5e", "v5litepod"):
    return "ct5lp-hightpu-1t" if chips == 1 else (
        "ct5lp-hightpu-8t" if chips == 8 else "ct5lp-hightpu-4t"
    )
  if family == "v4":
    return "ct4p-hightpu-4t"
  if family == "v5p":
    return "ct5p-hightpu-4t"
  return "tpu7x-standard-4t"



# Authoritative Go mappings matching pkg/config/hardware.go AcceleratorShorthandMap
STATIC_ACCELERATOR_SHORTHAND_MAP: dict[str, str] = {
    # GPU mappings
    "l4-1": "g2-standard-12",
    "l4-2": "g2-standard-24",
    "l4-4": "g2-standard-48",
    "l4-8": "g2-standard-96",
    "rtx-6000-1": "g4-standard-48",
    "rtx-6000-2": "g4-standard-96",
    "rtx-6000-4": "g4-standard-192",
    "rtx-6000-8": "g4-standard-384",
    "a100-40gb-1": "a2-highgpu-1g",
    "a100-40gb-2": "a2-highgpu-2g",
    "a100-40gb-4": "a2-highgpu-4g",
    "a100-40gb-8": "a2-highgpu-8g",
    "a2-megagpu-16g": "a2-megagpu-16g",
    "a100-80gb-1": "a2-ultragpu-1g",
    "a100-80gb-2": "a2-ultragpu-2g",
    "a100-80gb-4": "a2-ultragpu-4g",
    "a100-80gb-8": "a2-ultragpu-8g",
    "h100-80gb-1": "a3-highgpu-1g",
    "h100-80gb-2": "a3-highgpu-2g",
    "h100-80gb-4": "a3-highgpu-4g",
    "h100-80gb-8": "a3-highgpu-8g",
    "h100-mega-80gb-8": "a3-megagpu-8g",
    "h200-141gb-8": "a3-ultragpu-8g",
    "b200-8": "a4-highgpu-8g",
    "gb200-4": "a4x-highgpu-4g",
    # TPU mappings
    "v4-8": "ct4p-hightpu-4t",
    "v5p-8": "ct5p-hightpu-4t",
    "v5litepod-1": "ct5lp-hightpu-1t",
    "v5litepod-4": "ct5lp-hightpu-4t",
    "v5litepod-8": "ct5lp-hightpu-8t",
    "v6e-1": "ct6e-standard-1t",
    "v6e-4": "ct6e-standard-4t",
    "v6e-8": "ct6e-standard-8t",
    "tpu7x": "tpu7x-standard-4t",
}

# 3D topologies matching pkg/config/hardware.go common3DTopologies (keyed by total chips)
COMMON_3D_TOPOLOGIES: dict[int, str] = {
    4: "2x2x1",
    8: "2x2x2",
    16: "2x2x4",
    32: "2x4x4",
    64: "4x4x4",
    128: "4x4x8",
    256: "4x8x8",
    512: "8x8x8",
    1024: "8x8x16",
    2048: "8x16x16",
}

# 2D topologies matching pkg/config/hardware.go allowed2DTopologies (keyed by total chips)
ALLOWED_2D_TOPOLOGIES: dict[int, str] = {
    1: "1x1",
    4: "2x2",
    8: "2x4",
    16: "4x4",
    32: "4x8",
    64: "8x8",
    128: "8x16",
    256: "16x16",
}


def get_machine_type(device_type: str | None) -> tuple[str, str]:
  """Resolves GCE machine type and TPU topology from XPK device type.

  Returns (machine_type, topology). If topology is not applicable, returns (machine_type, 'N/A').
  If topology cannot be determined, returns (machine_type, 'UNKNOWN').
  """
  if not device_type:
    return "UNKNOWN", "N/A"

  d_lower = device_type.lower().strip()

  # 1. Reject deprecated TPU v2 and v3
  if d_lower.startswith("v2") or d_lower.startswith("v3"):
    return "ERROR: TPU v2/v3 are deprecated and not supported in Cluster Toolkit.", "N/A"

  # 2. Bare tpu7x or tpu7x- requires explicit topology
  if d_lower in ("tpu7x", "tpu7x-"):
    return "tpu7x-standard-4t", "UNKNOWN"

  # 3. Check for explicit dimension topology (e.g., tpu7x-4x4x8, v6e-1x1, v6e-4x4, v4-2x2x4)
  top_match = re.match(
      r"^(tpu7x|v6e|v5p|v5e|v5litepod|v4)-(\d+x\d+(?:x\d+)?)$",
      device_type,
      re.IGNORECASE,
  )
  if top_match:
    family, topology = top_match.group(1).lower(), top_match.group(2)
    try:
      dims = [int(x) for x in topology.split("x")]
      chips = math.prod(dims)
    except ValueError:
      chips = 0
    m_type = _tpu_machine_type(family, chips)
    return m_type, topology

  # 4. Check for chip/core count (e.g. v6e-1, v6e-16, v4-8, v4-128, v5p-128, tpu7x-128)
  chip_match = re.match(
      r"^(tpu7x|v6e|v5p|v5e|v5litepod|v4)-(\d+)$",
      device_type,
      re.IGNORECASE,
  )
  if chip_match:
    family, suffix_val = chip_match.group(1).lower(), int(chip_match.group(2))
    if family in ("v6e", "v5e", "v5litepod"):
      m_type = _tpu_machine_type(family, suffix_val)
      topology = ALLOWED_2D_TOPOLOGIES.get(suffix_val, "UNKNOWN")
      return m_type, topology
    elif family in ("v4", "v5p"):
      # Suffix represents TensorCores; divide by 2 to get chips
      chips = suffix_val // 2
      m_type = _tpu_machine_type(family, chips)
      topology = COMMON_3D_TOPOLOGIES.get(chips, "UNKNOWN")
      return m_type, topology
    elif family == "tpu7x":
      # Suffix represents chips (e.g., tpu7x-128 = 128 chips = 4x4x8)
      m_type = _tpu_machine_type(family, suffix_val)
      topology = COMMON_3D_TOPOLOGIES.get(suffix_val, "UNKNOWN")
      return m_type, topology

  # 5. Check for literal match in static accelerator shorthand map (GPU & fixed TPUs)
  if d_lower in STATIC_ACCELERATOR_SHORTHAND_MAP:
    return STATIC_ACCELERATOR_SHORTHAND_MAP[d_lower], "N/A"

  return device_type, "N/A"


def is_tpu_hardware(compute_type: str | None, device_type: str | None) -> bool:
  """Returns True if the compute hardware matches TPU prefix patterns."""
  c_lower = (compute_type or "").lower()
  d_lower = (device_type or "").lower()
  if "tpu" in c_lower or "tpu" in d_lower:
    return True
  pattern = r"^(v[4-9]|ct[4-9]|v5litepod)"
  return bool(re.search(pattern, c_lower) or re.search(pattern, d_lower))


def resolve_is_tpu(
    device_type: str | None,
    m_type: str | None,
    explicit_tpu_flag: bool = False,
) -> bool:
  """Determines whether the device or machine type represents TPU hardware."""
  if is_tpu_hardware(m_type, device_type):
    return True
  if explicit_tpu_flag:
    d_str = (device_type or "").lower()
    m_str = (m_type or "").lower()
    is_known_gpu = (
        any(k in d_str or k in m_str for k in KNOWN_GPU_SUBSTRINGS)
        or (d_str in STATIC_ACCELERATOR_SHORTHAND_MAP)
        or (m_str in STATIC_ACCELERATOR_SHORTHAND_MAP)
    )
    if not is_known_gpu:
      return True
  return False



def is_unified_gpu_hardware(
    compute_type: str | None, device_type: str | None
) -> bool:
  """Returns True if hardware matches modern unified GPU families (A3 Mega/High/Ultra, A4)."""
  text = f"{compute_type or ''} {device_type or ''}".lower()
  if "a4x" in text:
    return False
  patterns = [
      r"\ba3-",
      r"\ba4-",
      r"\bh100",
      r"\bh200",
      r"\bb200",
  ]
  return any(re.search(p, text) for p in patterns)


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
      # Not registered in xpk v1.16.0 (--zone's help text at
      # xpk/src/xpk/parser/common.py:106 references --region as a planned
      # alternative). Accepted defensively so forward-compatible and
      # partially-migrated commands still translate.
      "--region": "--location",
      "--location": "--location",
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
      # Identity passthroughs for gcluster-native flags. These are not xpk flags;
      # they are kept so a partially-migrated command retains flags a user has
      # already converted by hand. Do not "fix" them by removing them.
      "--service-account": "--service-account",
      "--service-account-name": "--service-account",
      "--image-pull-secret": "--image-pull-secret",
      "--docker-image-pull-secret": "--image-pull-secret",
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
      "--colocated-python-sidecar-image": (
          "--pathways-colocated-python-sidecar-image"
      ),
      "--cpu-affinity": "--cpu-affinity",
      "--gke-custom-templates-path": "--gke-custom-templates-path",
      "--gke-nap-provisioning": "--gke-nap-provisioning",
      "--gke-nap-reservation": "--gke-nap-reservation",
      "--pathways-head-np": "--pathways-head-np",
      "--pathways-worker-image": "--pathways-worker-image",
      "--platform": "--platform",
  }

  boolean_flags = {
      "--wait-for-job-completion": "--await-job-completion",
      "--mtc-enabled": "--gke-mtc-enabled",
      "--headless": "--pathways-headless",
      "--enable-debug-logs": "--verbose",
      "--deploy-stacktrace-sidecar": "--verbose",
      "--skip-prereqs": "--skip-prereqs",
  }

  parser = argparse.ArgumentParser(allow_abbrev=False)
  for flag in value_flags.keys():
    parser.add_argument(flag, type=str, nargs="?")
  for flag in boolean_flags.keys():
    parser.add_argument(flag, nargs="?", const="true")

  parser.add_argument("--spot", nargs="?", const="true")
  parser.add_argument("--on-demand", nargs="?", const="true")
  parser.add_argument("--flex", nargs="?", const="true")
  parser.add_argument("--reservation", type=str)
  parser.add_argument("--tpu-type", type=str)
  parser.add_argument("--device-type", type=str)
  parser.add_argument("--use-parallel-containers", type=str, nargs="?")
  parser.add_argument("--mount-options", type=str)
  parser.add_argument("--storage-options", type=str)
  parser.add_argument("--env", action="append", default=[])
  parser.add_argument("--env-file", type=str)
  parser.add_argument("--storage", action="append", default=[])
  parser.add_argument("--pathways-proxy-env", action="append", default=[])
  parser.add_argument("--pathways-server-env", action="append", default=[])
  parser.add_argument("--pathways-worker-env", action="append", default=[])

  parsed, rest = parser.parse_known_args(unknown)

  if parsed.env_file:
    warnings.append(
        "`--env-file` is not directly supported by `gcluster job submit`. Expand"
        " the file's contents into repeated `--env KEY=VALUE` flags instead."
    )

  device_type = parsed.tpu_type or parsed.device_type
  m_type, top = (None, None)
  if device_type:
    m_type, top = get_machine_type(device_type)

  is_tpu = resolve_is_tpu(device_type, m_type, bool(parsed.tpu_type))

  for flag, mapped_flag in value_flags.items():
    val = getattr(parsed, flag.lstrip("-").replace("-", "_"), None)
    if val is not None:
      if mapped_flag == "--num-nodes" and is_tpu:
        warnings.append(
            "Omitted --num-nodes because it is not supported for TPU jobs in"
            " gcluster."
        )
        continue
      if mapped_flag == "--base-image":
        if parsed.docker_image is not None:
          continue
        if parsed.script_dir is not None:
          cmd.extend(["--base-image", val, "--build-context", parsed.script_dir])
        else:
          # In XPK, --base-docker-image without --script-dir simply designates the container image
          cmd.extend(["--image", val])
        continue
      if mapped_flag == "--build-context":
        if parsed.docker_image is not None:
          continue
        # Handled alongside --base-image or when explicit
        if parsed.base_docker_image is None:
          cmd.extend(["--base-image", "python:3.10", "--build-context", val])
          warnings.append(
              "`--script-dir` was specified without `--base-docker-image` or"
              " `--docker-image`. In xpk, this defaults to `python:3.10`."
              " Emitted `--base-image python:3.10` alongside `--build-context`"
              " to satisfy gcluster's requirement that `--build-context` must"
              " be paired with `--base-image`."
          )
        continue
      if mapped_flag == "--image":
        if parsed.base_docker_image is not None or parsed.script_dir is not None:
          warnings.append(
              "`--docker-image` cannot be combined with `--base-docker-image` or"
              " `--script-dir`; xpk rejects this combination (exit 1). Emitting"
              " `--image` only, per xpk's precedence (--docker-image is used"
              " directly). Re-check that this was the intended source command."
          )
        cmd.extend(["--image", val])
        continue
      if mapped_flag == "--restart-on-exit-codes":
        raw_codes = [c.strip() for c in val.split(",") if c.strip()]
        valid_codes = []
        seen = set()
        for c in raw_codes:
          try:
            icode = int(c)
            if 1 <= icode <= 255:
              if icode not in seen:
                seen.add(icode)
                valid_codes.append(str(icode))
            else:
              warnings.append(
                  f"Invalid exit code '{c}' in --restart-on-exit-codes. Valid"
                  " Kubernetes exit codes must be between 1 and 255."
              )
          except ValueError:
            if c not in seen:
              seen.add(c)
              valid_codes.append(c)
        if valid_codes:
          cmd.extend(["--restart-on-exit-codes", ",".join(valid_codes)])
        continue
      if mapped_flag.startswith("--pathways-") and not is_pathways:
        warnings.append(
            f"{mapped_flag} is only supported for Pathways workloads (--pathways); flag omitted."
        )
        continue
      cmd.extend([mapped_flag, val])

  for flag, mapped_flag in boolean_flags.items():
    raw_val = getattr(parsed, flag.lstrip("-").replace("-", "_"), None)
    if is_flag_true(raw_val):
      if mapped_flag.startswith("--pathways-") and not is_pathways:
        warnings.append(
            f"{mapped_flag} is only supported for Pathways workloads (--pathways); flag omitted."
        )
        continue
      cmd.append(mapped_flag)

  nap_prov = getattr(parsed, "gke_nap_provisioning", None)
  nap_res = getattr(parsed, "gke_nap_reservation", None)

  consumption_count = sum([
      is_flag_true(parsed.spot),
      is_flag_true(parsed.on_demand),
      bool(parsed.reservation),
  ])
  if consumption_count > 1:
    warnings.append(
        "Conflicting workload consumption options specified: --spot, --on-demand,"
        " and --reservation are mutually exclusive. Ensure only one consumption"
        " model is specified."
    )

  if is_flag_true(parsed.spot):
    if not nap_prov:
      cmd.extend(["--gke-nap-provisioning", "spot"])
      nap_prov = "spot"
      warnings.append(
          "--spot was mapped to --gke-nap-provisioning spot (supported on GKE"
          " clusters with Node Auto-Provisioning enabled). For static node pool"
          " clusters, spot consumption is configured in the node pool blueprint."
      )
  elif is_flag_true(parsed.on_demand):
    if not nap_prov:
      cmd.extend(["--gke-nap-provisioning", "on-demand"])
      nap_prov = "on-demand"
      warnings.append(
          "--on-demand was mapped to --gke-nap-provisioning on-demand"
          " (supported on GKE clusters with Node Auto-Provisioning enabled)."
      )
  elif parsed.reservation:
    if not nap_prov:
      cmd.extend([
          "--gke-nap-provisioning",
          "reservation",
          "--gke-nap-reservation",
          parsed.reservation,
      ])
      nap_prov = "reservation"
      warnings.append(
          f"--reservation was mapped to --gke-nap-provisioning reservation"
          f" --gke-nap-reservation {parsed.reservation} (supported on GKE"
          " clusters with Node Auto-Provisioning enabled). For static node pool"
          " clusters, reservation affinity is configured in the node pool"
          " blueprint."
      )

  if nap_prov and nap_prov.lower() == "reservation" and not nap_res and not parsed.reservation:
    warnings.append(
        "--gke-nap-reservation is required when --gke-nap-provisioning=reservation."
    )

  if is_flag_true(parsed.flex):
    warnings.append(
        "--flex is a cluster/node pool infrastructure provisioning setting in"
        " Cluster Toolkit (enable_flex_start on the node pool). For workload"
        " submission to DWS flex start, submit with --queue dws-queue."
    )

  if parsed.workload:
    w_name = parsed.workload.strip("'\"")
    if "$" not in w_name and "<" not in w_name:
      if is_pathways and len(w_name) > 22:
        warnings.append(
            f"Workload name '{w_name}' exceeds 22 characters ({len(w_name)})."
            " Pathways workloads require names <= 22 characters due to the"
            " Kubernetes 63-byte coordinator label limit"
            " (<name>-pathways-head-0-0.<name>)."
        )
      elif len(w_name) > 28:
        warnings.append(
            f"Workload name '{w_name}' exceeds 28 characters ({len(w_name)})."
            " Workload names should be <= 28 characters to prevent Kubernetes"
            " DNS and JobSet child label truncation."
        )

  if is_pathways and not getattr(parsed, "pathways_gcs_location", None):
    warnings.append(
        "When submitting Pathways workloads (--pathways), --pathways-gcs-location"
        " is required by gcluster."
    )

  if device_type:
    if "ERROR" in str(m_type):
      warnings.append(str(m_type))
    elif "$" in device_type or "<" in device_type:
      if device_type.lower().startswith("tpu7x"):
        cmd.extend([
            "--compute-type",
            "tpu7x-standard-4t",
            "--topology",
            "<YOUR_TOPOLOGY>",
        ])
        warnings.append(
            f"TPU 7x requires an explicit --topology for '{device_type}'."
            " Please replace '<YOUR_TOPOLOGY>' with your slice topology."
        )
      elif is_tpu:
        cmd.extend([
            "--compute-type",
            device_type,
            "--topology",
            "<YOUR_TOPOLOGY>",
        ])
        warnings.append(
            "TPU workloads require an explicit --topology when using variable"
            f" '{device_type}'. Please replace '<YOUR_TOPOLOGY>' with your"
            " slice topology."
        )
      else:
        cmd.extend(["--compute-type", device_type])
    elif is_tpu:
      d_lower = device_type.lower()
      if d_lower in STATIC_ACCELERATOR_SHORTHAND_MAP and d_lower != "tpu7x":
        cmd.extend(["--compute-type", device_type])
      elif top == "UNKNOWN":
        if d_lower.startswith("tpu7x"):
          warnings.append(
              f"Could not determine TPU 7x topology for '{device_type}'. Valid"
              f" chip counts are {sorted(COMMON_3D_TOPOLOGIES.keys())}. Please"
              " pass an explicit --topology (e.g. --topology=AxBxC)."
          )
          cmd.extend([
              "--compute-type",
              "tpu7x-standard-4t",
              "--topology",
              "<YOUR_TOPOLOGY>",
          ])
        else:
          warnings.append(
              f"Could not determine topology for TPU '{device_type}'. Please"
              " specify --topology explicitly."
          )
          cmd.extend(
              ["--compute-type", m_type, "--topology", "<YOUR_TOPOLOGY>"]
          )
      else:
        cmd.extend(["--compute-type", m_type])
        if top and top != "N/A":
          cmd.extend(["--topology", top])
    else:
      d_lower = device_type.lower()
      if d_lower in STATIC_ACCELERATOR_SHORTHAND_MAP:
        cmd.extend(["--compute-type", device_type])
      else:
        cmd.extend(["--compute-type", m_type])
        if (
            m_type == device_type
            and not d_lower.startswith(("$", "<"))
            and not GCE_MACHINE_TYPE_RE.match(d_lower)
        ):
          warnings.append(
              f"'{device_type}' is not a known accelerator shorthand in Cluster"
              " Toolkit. Please specify a valid GCE machine type or mapped GPU"
              " shorthand."
          )

  if (
      parsed.use_parallel_containers is not None
      and str(parsed.use_parallel_containers).lower() == "false"
  ):
    cmd.append("--gke-disable-parallel-containers")

  mount_options = parsed.mount_options or parsed.storage_options

  for env_val in parsed.env:
    cmd.extend(["--env", env_val])
  seen_dests: dict[str, int] = {}
  for storage_val in parsed.storage:
    if storage_val.startswith(("s3://", "http://", "https://", "ftp://")):
      warnings.append(
          f"Storage URI '{storage_val}' uses an unsupported scheme. Cluster"
          " Toolkit supports Cloud Storage ('gs://'), Filestore"
          " ('filestore://'), or PersistentVolumeClaims."
      )
    if ";" in storage_val:
      parts = storage_val.split(";")
      dest = parts[1] if len(parts) > 1 else ""
      if dest:
        count = seen_dests.get(dest, 0) + 1
        seen_dests[dest] = count
        if count > 1:
          parts[1] = f"{dest}_{count}"
          storage_val = ";".join(parts)
      if mount_options and not any(p.startswith("options=") for p in parts):
        if parts[0].startswith("gs://"):
          parts.append(f"options={mount_options}")
          storage_val = ";".join(parts)
      cmd.extend(["--mount", storage_val])
    else:
      last_component = storage_val.rstrip("/").split("/")[-1]
      dest_name = re.sub(r"[\$\{\}\<\>]", "", last_component).strip().lower()
      clean_dest = dest_name if dest_name else "storage"
      base_dest = f"/mnt/{clean_dest}"
      count = seen_dests.get(base_dest, 0) + 1
      seen_dests[base_dest] = count
      final_dest = base_dest if count == 1 else f"{base_dest}_{count}"
      mount_spec = f"{storage_val};{final_dest};ro"
      if mount_options and storage_val.startswith("gs://"):
        mount_spec += f";options={mount_options}"
      warnings.append(
          f"`--storage {storage_val}` references an xpk Storage object; the"
          " mount point, PVC name, and read-only mode live on that object, not"
          f" on the workload command. Emitted `{mount_spec}` as a placeholder."
          " Recover the real values with `xpk storage list` (or `gcluster"
          " cluster volume`) and correct src, dest, and mode before"
          " submitting."
      )
      cmd.extend(["--mount", mount_spec])
  if is_pathways:
    for env_val in parsed.pathways_proxy_env:
      cmd.extend(["--pathways-proxy-env", env_val])
    for env_val in parsed.pathways_server_env:
      cmd.extend(["--pathways-server-env", env_val])
    for env_val in parsed.pathways_worker_env:
      cmd.extend(["--pathways-worker-env", env_val])
  elif (
      parsed.pathways_proxy_env
      or parsed.pathways_server_env
      or parsed.pathways_worker_env
  ):
    warnings.append(
        "Pathways environment variables (--pathways-*-env) were specified but"
        " --pathways was not enabled; ignoring these flags."
    )

  return cmd, warnings, rest


def parse_cluster_create(
    is_pathways: bool, unknown: list[str]
) -> tuple[dict[str, Any], list[str], list[str]]:
  """Parses XPK cluster create arguments into Cluster Toolkit blueprint data structure."""
  parser = argparse.ArgumentParser(allow_abbrev=False)
  parser.add_argument("--cluster", type=str)
  parser.add_argument("--project", type=str)
  parser.add_argument("--zone", type=str)
  parser.add_argument("--region", type=str)
  parser.add_argument("--num-slices", type=str)
  parser.add_argument("--tpu-type", type=str)
  parser.add_argument("--device-type", type=str)
  parser.add_argument("--default-pool-cpu-machine-type", type=str)
  parser.add_argument("--cluster-cpu-machine-type", type=str)
  parser.add_argument("--reservation", type=str)
  parser.add_argument(
      "--default-pool-cpu-num-nodes",
      "--system-node-pool-node-count",
      "--system-pool-node-count",
      dest="system_node_pool_node_count",
      type=str,
  )
  parser.add_argument("--gke-version", type=str)
  parser.add_argument("--authorized-networks", type=str)
  parser.add_argument("--private-endpoint-subnetwork", type=str)
  parser.add_argument("--num-nodes", type=str)
  parser.add_argument("--sub-slicing", nargs="?", const="true")
  parser.add_argument("--super-slicing", nargs="?", const="true")
  parser.add_argument("--num-cubes", type=str)
  parser.add_argument("--autoprovisioning-min-chips", type=str)
  parser.add_argument("--autoprovisioning-max-chips", type=str)
  parser.add_argument("--host-maintenance-interval", type=str)
  parser.add_argument("--maintenance-interval", type=str)
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
      "--enable-filestore-csi-driver",
      "--enable-parallelstore-csi-driver",
      "--enable-pd-csi-driver",
      "--enable-managed-disk-csi-driver",
      "--enable-persistent-disk-csi-driver",
      "--flex",
      "--enable-autoprovisioning",
      "--enable-pathways",
      "--enable-mtc",
      "--create-vertex-tensorboard",
      "--managed-mldiagnostics",
      "--enable-ml-diagnostics",
      "--enable-mldiagnostics",
  ]
  for flag in boolean_flags:
    parser.add_argument(flag, nargs="?", const="true")

  parsed, rest = parser.parse_known_args(unknown)

  deployment_name = parsed.cluster or "my-cluster"

  # Top-level global vars that canonical blueprints expect in vars:
  blueprint_vars: dict[str, Any] = {
      "deployment_name": deployment_name,
      "project_id": parsed.project or "<YOUR_PROJECT_ID>",
  }

  cli_region = None
  if parsed.region:
    blueprint_vars["region"] = parsed.region
    cli_region = parsed.region
    blueprint_vars["zone"] = parsed.zone or "<YOUR_ZONE>"
  elif parsed.zone and parsed.zone.startswith("$"):
    blueprint_vars["zone"] = parsed.zone
    var_match = re.match(r"^\$\{?([a-zA-Z_][a-zA-Z0-9_]*)", parsed.zone)
    if var_match:
      var_name = var_match.group(1)
      cli_region = f"${{{var_name}%-*}}"
    blueprint_vars["region"] = "<YOUR_REGION>"
  elif parsed.zone and "-" in parsed.zone:
    blueprint_vars["zone"] = parsed.zone
    blueprint_vars["region"] = parsed.zone.rsplit("-", 1)[0]
  else:
    blueprint_vars["zone"] = parsed.zone or "<YOUR_ZONE>"
    blueprint_vars["region"] = "<YOUR_REGION>"

  if parsed.num_slices:
    try:
      blueprint_vars["num_slices"] = int(parsed.num_slices)
    except ValueError:
      blueprint_vars["num_slices"] = parsed.num_slices

  # Module settings for modules/scheduler/gke-cluster (to prevent collision with explicit module settings)
  cluster_module_settings: dict[str, Any] = {}
  # Module settings for modules/compute/gke-node-pool
  nodepool_module_settings: dict[str, Any] = {}

  warnings: list[str] = []
  device_type = parsed.tpu_type or parsed.device_type
  if device_type:
    m_type, top = get_machine_type(device_type)
    if "ERROR" in str(m_type):
      warnings.append(str(m_type))
    else:
      is_tpu = resolve_is_tpu(device_type, m_type, bool(parsed.tpu_type))
      blueprint_vars["machine_type"] = m_type
      if top and top not in ("N/A", "UNKNOWN"):
        blueprint_vars["tpu_topology"] = top
      elif is_tpu:
        blueprint_vars["tpu_topology"] = "<YOUR_TOPOLOGY>"
        if (top == "UNKNOWN" or top == "N/A") and device_type.lower().startswith("tpu7x"):
          warnings.append(
              f"Could not determine TPU 7x topology for '{device_type}'. Valid"
              f" chip counts: {sorted(COMMON_3D_TOPOLOGIES.keys())}. Please"
              " specify tpu_topology manually."
          )

      if (
          m_type == device_type
          and not (device_type or "").lower().startswith(("$", "<"))
          and not is_tpu
          and not GCE_MACHINE_TYPE_RE.match((device_type or "").lower())
      ):
        warnings.append(
            f"'{device_type}' is not a known accelerator shorthand in Cluster"
            " Toolkit. Please specify a valid GCE machine type or mapped GPU"
            " shorthand."
        )

  if parsed.authorized_networks:
    cidrs = [c.strip() for c in parsed.authorized_networks.split(",") if c.strip()]
    if len(cidrs) > 1:
      # The canonical blueprints' only consumer of $(vars.authorized_cidr) is
      # this list, so the first entry must keep referencing it. Inlining every
      # CIDR would orphan the global variable and trip
      # testDeploymentVariableNotUsed.
      cluster_module_settings["master_authorized_networks"] = [
          {
              "cidr_block": "$(vars.authorized_cidr)" if i == 0 else c,
              "display_name": f"access-net-{i+1}",
          }
          for i, c in enumerate(cidrs)
      ]
      blueprint_vars["authorized_cidr"] = cidrs[0]
    elif cidrs:
      blueprint_vars["authorized_cidr"] = cidrs[0]

  # Validate mutual exclusivity of consumption options
  consumption_count = sum([
      is_flag_true(parsed.spot),
      is_flag_true(parsed.flex),
      bool(parsed.reservation),
      is_flag_true(parsed.on_demand),
  ])
  if consumption_count > 1:
    warnings.append(
        "Conflicting consumption options specified: spot, flex start, on-demand,"
        " and specific reservations are mutually exclusive in GKE node pools."
        " Ensure only one consumption model is active."
    )

  is_unified = is_unified_gpu_hardware(
      blueprint_vars.get("machine_type"), device_type
  )

  if is_unified:
    if parsed.reservation:
      blueprint_vars["reservation_affinity"] = {
          "consume_reservation_type": "SPECIFIC_RESERVATION",
          "specific_reservations": [{"name": parsed.reservation}],
      }
      warnings.append(
          "For modern unified GPU blueprints (A3 Mega/High/Ultra, A4),"
          " consumption models are configured directly in `vars:` via"
          " `reservation_affinity: {consume_reservation_type:"
          f" 'SPECIFIC_RESERVATION', specific_reservations: [{{name: '{parsed.reservation}'}}]}}`."
          " Note: reservation_affinity is an object and cannot be passed via"
          " `gcluster deploy --vars`; configure it directly in the blueprint"
          " YAML."
      )
    elif is_flag_true(parsed.spot):
      blueprint_vars["spot"] = True
      warnings.append(
          "Spot consumption configured directly in `vars:` with `spot: true`"
          " for unified GPU blueprints. (In modern unified GPU blueprints,"
          " `vars.reservation_affinity` already defaults to NO_RESERVATION)."
      )
    elif is_flag_true(parsed.flex):
      blueprint_vars["enable_flex_start"] = True
      blueprint_vars["static_node_count"] = "$(null)"
      warnings.append(
          "DWS Flex Start configured directly in `vars:` with"
          " `enable_flex_start: true` and `static_node_count: $(null)` for"
          " unified GPU blueprints. (In modern unified GPU blueprints,"
          " `auto_repair` is computed automatically from"
          " `vars.enable_flex_start` and `vars.reservation_affinity` already"
          " defaults to NO_RESERVATION)."
      )
      if parsed.num_nodes:
        warnings.append(
            "enable_flex_start requires static_node_count to be null;"
            " static_node_count has been set to $(null). User sizing intent"
            f" ({parsed.num_nodes} nodes) can be preserved by setting"
            f" `autoscaling_total_max_nodes: {parsed.num_nodes}` on the node pool"
            " module when dynamic autoscaling or queued provisioning is desired."
        )
    elif is_flag_true(parsed.on_demand):
      warnings.append(
          "Default on-demand consumption active (in modern unified GPU"
          " blueprints, `vars.reservation_affinity` defaults to NO_RESERVATION"
          " and `vars.spot` defaults to false)."
      )
  else:
    if parsed.reservation:
      blueprint_vars["reservation"] = parsed.reservation
      nodepool_module_settings["reservation_affinity"] = {
          "consume_reservation_type": "SPECIFIC_RESERVATION",
          "specific_reservations": [{"name": "$(vars.reservation)"}],
      }
      warnings.append(
          "For TPU and specialized blueprints (A4X, G4, H4D), reservation"
          " affinity is configured via `reservation_affinity:"
          " {consume_reservation_type: 'SPECIFIC_RESERVATION',"
          " specific_reservations: [{name: $(vars.reservation)}]}` on"
          " the node pool module, parameterized by `reservation: "
          f"{parsed.reservation}` in vars:. Note: reservation_affinity is an object"
          " and cannot be passed via `gcluster deploy --vars`; configure it"
          " in the blueprint YAML while setting vars.reservation."
      )
    elif is_flag_true(parsed.spot):
      nodepool_module_settings["spot"] = True
      nodepool_module_settings["reservation_affinity"] = dict(NO_RESERVATION_AFFINITY)
      warnings.append(
          "Spot consumption configured on node pool with reservation_affinity:"
          " {consume_reservation_type: 'NO_RESERVATION', specific_reservations: []}."
      )
    elif is_flag_true(parsed.flex):
      nodepool_module_settings["enable_flex_start"] = True
      nodepool_module_settings["auto_repair"] = False
      nodepool_module_settings["static_node_count"] = "$(null)"
      nodepool_module_settings["reservation_affinity"] = dict(NO_RESERVATION_AFFINITY)
      warnings.append(
          "DWS Flex Start configured on node pool with auto_repair: false,"
          " static_node_count: $(null), and reservation_affinity:"
          " {consume_reservation_type: 'NO_RESERVATION', specific_reservations: []}."
      )
      if parsed.num_nodes:
        warnings.append(
            "enable_flex_start requires static_node_count to be null;"
            " static_node_count has been set to $(null). User sizing intent"
            f" ({parsed.num_nodes} nodes) can be preserved by setting"
            f" `autoscaling_total_max_nodes: {parsed.num_nodes}` on the node pool"
            " module when dynamic autoscaling or queued provisioning is desired."
        )
    elif is_flag_true(parsed.on_demand):
      nodepool_module_settings["reservation_affinity"] = dict(NO_RESERVATION_AFFINITY)
      warnings.append(
          "On-demand consumption without reservations is configured via"
          " `reservation_affinity: {consume_reservation_type: 'NO_RESERVATION',"
          " specific_reservations: []}` on the node pool module."
      )

  if is_pathways or is_flag_true(parsed.enable_pathways):
    blueprint_vars["enable_pathways_for_tpus"] = True
    warnings.append(
        "enable_pathways_for_tpus: true automatically provisions the dedicated"
        " cpu-np CPU node pool with default n4-standard-64 instances."
    )
  elif is_tpu_hardware(blueprint_vars.get("machine_type"), device_type):
    # Canonical TPU blueprints default this to true; without an explicit
    # false the merge leaves an unrequested cpu-np pool in place.
    # GPU blueprints do not declare this variable, so emitting it there
    # would trip testDeploymentVariableNotUsed.
    blueprint_vars["enable_pathways_for_tpus"] = False
    warnings.append(
        "enable_pathways_for_tpus: false emitted explicitly to override the"
        " `true` default in canonical TPU blueprints."
    )

  if is_flag_true(parsed.sub_slicing) or is_flag_true(parsed.super_slicing):
    # Only the gke-tpu-7x blueprints declare this variable; emitting it
    # elsewhere would trip testDeploymentVariableNotUsed.
    if str(device_type or "").lower().startswith("tpu7x"):
      blueprint_vars["enable_dynamic_slicing_for_tpus"] = True
      # accelerator_topology_mode: PROVISION_ONLY is only valid alongside
      # reservation_affinity.consume_reservation_type == SPECIFIC_RESERVATION
      # (modules/compute/gke-node-pool/main.tf:482). --spot, --flex and
      # --on-demand all force NO_RESERVATION, so emitting it there would
      # produce a blueprint that cannot pass `terraform plan`.
      if parsed.reservation:
        nodepool_module_settings["accelerator_topology_mode"] = "PROVISION_ONLY"
      else:
        warnings.append(
            "CONFLICT: dynamic slicing requires nodepool"
            " accelerator_topology_mode: PROVISION_ONLY, which Cluster Toolkit"
            " only permits when reservation_affinity.consume_reservation_type"
            " is SPECIFIC_RESERVATION (gke-node-pool/main.tf:482). --spot,"
            " --flex and --on-demand all force NO_RESERVATION, so they cannot"
            " be combined with --sub-slicing/--super-slicing."
            " accelerator_topology_mode was omitted; supply --reservation or"
            " drop the slicing flag."
        )
      warnings.append(
          "enable_dynamic_slicing_for_tpus: true requires GKE >= "
          "1.35.0-gke.274500, nodepool accelerator_topology_mode:"
          " PROVISION_ONLY, and kueue.install and jobset.install enabled."
      )
    else:
      cluster_module_settings["enable_slice_controller"] = True
      warnings.append(
          "This blueprint does not declare enable_dynamic_slicing_for_tpus; "
          "enable_slice_controller was set on the gke-cluster module instead. "
          "The Kueue dynamic-slicing configuration is not wired up here and "
          "must be added manually."
      )

  if parsed.num_cubes:
    # xpk treats --num-cubes as an alias for --num-slices: it rejects the two
    # being different and assigns num_slices = num_cubes when only the latter
    # is given (xpk/src/xpk/commands/cluster.py:348-363).
    if not is_flag_true(parsed.super_slicing):
      warnings.append(
          "--num-cubes can only be used with --super-slicing in xpk; xpk exits"
          " with an error otherwise."
      )
    if parsed.num_slices and str(parsed.num_slices) != str(parsed.num_cubes):
      warnings.append(
          f"--num-cubes ({parsed.num_cubes}) differs from --num-slices"
          f" ({parsed.num_slices}); xpk rejects this combination. Using"
          " --num-slices for num_slices."
      )
    elif not parsed.num_slices:
      try:
        blueprint_vars["num_slices"] = int(parsed.num_cubes)
      except ValueError:
        blueprint_vars["num_slices"] = parsed.num_cubes
      warnings.append(
          "--num-cubes maps to num_slices (xpk sets num_slices = num_cubes;"
          " cluster.py:361)."
      )
  if (
      parsed.num_nodes
      and not is_flag_true(parsed.flex)
      and not is_tpu_hardware(
          blueprint_vars.get("machine_type"), device_type
      )
  ):
    try:
      blueprint_vars["static_node_count"] = int(parsed.num_nodes)
    except ValueError:
      blueprint_vars["static_node_count"] = parsed.num_nodes

  cpu_machine_type = (
      parsed.default_pool_cpu_machine_type or parsed.cluster_cpu_machine_type
  )
  if cpu_machine_type:
    cluster_module_settings["system_node_pool_machine_type"] = cpu_machine_type

  if parsed.system_node_pool_node_count:
    try:
      cnt = int(parsed.system_node_pool_node_count)
      cluster_module_settings["system_node_pool_node_count"] = {
          "total_min_nodes": cnt,
          "total_max_nodes": cnt,
      }
    except ValueError:
      cluster_module_settings["system_node_pool_node_count"] = {
          "total_min_nodes": parsed.system_node_pool_node_count,
          "total_max_nodes": parsed.system_node_pool_node_count,
      }

  if is_flag_true(parsed.private) or is_flag_true(
      parsed.enable_private_endpoint
  ):
    cluster_module_settings["enable_private_endpoint"] = True

  if parsed.gke_version:
    if parsed.gke_version.endswith("."):
      cluster_module_settings["version_prefix"] = parsed.gke_version
    else:
      cluster_module_settings["min_master_version"] = parsed.gke_version

  boolean_cluster_mappings = [
      ("enable_master_global_access", "enable_master_global_access"),
      ("enable_workload_identity", "configure_workload_identity_sa"),
      ("enable_gcsfuse_csi_driver", "enable_gcsfuse_csi"),
      ("enable_lustre_csi_driver", "enable_managed_lustre_csi"),
      ("enable_parallelstore_csi_driver", "enable_parallelstore_csi"),
  ]
  for flag_attr, setting in boolean_cluster_mappings:
    if is_flag_true(getattr(parsed, flag_attr, None)):
      cluster_module_settings[setting] = True

  if (
      is_flag_true(parsed.enable_gcpfilestore_csi_driver)
      or is_flag_true(parsed.enable_filestore_csi_driver)
  ):
    cluster_module_settings["enable_filestore_csi"] = True

  if (
      is_flag_true(parsed.enable_pd_csi_driver)
      or is_flag_true(parsed.enable_managed_disk_csi_driver)
      or is_flag_true(parsed.enable_persistent_disk_csi_driver)
  ):
    cluster_module_settings["enable_persistent_disk_csi"] = True

  if (
      is_flag_true(parsed.enable_ml_diagnostics)
      or is_flag_true(parsed.enable_mldiagnostics)
      or is_flag_true(parsed.managed_mldiagnostics)
  ):
    cluster_module_settings["enable_ml_diagnostics"] = True
    # Note: no Terraform precondition couples ML diagnostics to workload
    # identity, so configure_workload_identity_sa is deliberately NOT enabled
    # here -- doing so would create a service account and IAM bindings the
    # user did not ask for.
    warnings.append(
        "enable_ml_diagnostics requires GKE >= the version named in"
        " gke-cluster/main.tf:796. Several canonical blueprints also set"
        " configure_workload_identity_sa: true alongside it; enable that"
        " separately if your workloads need it."
    )

  m_interval = parsed.host_maintenance_interval or parsed.maintenance_interval
  if m_interval:
    nodepool_module_settings["host_maintenance_interval"] = m_interval

  if is_flag_true(parsed.enable_mtc):
    cluster_module_settings["enable_multi_tier_checkpointing"] = True
    cluster_module_settings["enable_gcsfuse_csi"] = True
    warnings.append(
        "enable_gcsfuse_csi: true was enabled automatically because"
        " enable_multi_tier_checkpointing requires it"
        " (gke-cluster/main.tf:176-177)."
    )

  if parsed.private_endpoint_subnetwork:
    warnings.append(
        "private_endpoint_subnetwork is not a standard blueprint variable."
        " Specify custom subnets in the network module settings."
    )
  if (
      is_flag_true(parsed.enable_autoprovisioning)
      or parsed.autoprovisioning_min_chips
      or parsed.autoprovisioning_max_chips
  ):
    warnings.append(
        "Cluster autoscaling/autoprovisioning is configured via the"
        " cluster_autoscaling object in modules/scheduler/gke-cluster."
    )
  if (
      parsed.mtc_ramdisk_size
      or parsed.mtc_gcs_bucket
      or parsed.mtc_toleration_key
  ):
    warnings.append(
        "In Cluster Toolkit, MTC ramdisk directory and storage buckets are"
        " configured per workload at job submission via `gcluster job submit"
        " --gke-mtc-enabled --gke-mtc-ramdisk-dir <dir>` and `--mount`."
    )
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

  blueprint_name = deployment_name
  if blueprint_name.startswith("$") or not re.match(
      r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$", blueprint_name
  ):
    blueprint_name = "my-cluster"

  blueprint: dict[str, Any] = {
      "blueprint_name": blueprint_name,
      "vars": blueprint_vars,
      "cluster_module_settings": cluster_module_settings,
      "nodepool_module_settings": nodepool_module_settings,
      "cli_region": cli_region,
  }
  return blueprint, warnings, rest


def format_cluster_output(
    blueprint: dict[str, Any], warnings: list[str], unmapped_flags: list[str]
) -> str:
  """Formats cluster creation blueprint and commands into printable string."""
  result = ""
  for w in warnings:
    result += f"# Note: {w}\n"
  if unmapped_flags:
    flags_str = " ".join(unmapped_flags)
    result += (
        f"# Warning: Unmapped xpk flags were ignored: {flags_str}\n"
        "# The tool is not sure how to translate these unrecognized flag(s) into Cluster Toolkit.\n"
        "# Please verify whether an equivalent setting is needed in your Cluster Toolkit configuration or consult the migration guide.\n"
    )

  v = blueprint.get("vars", {})
  cluster_settings = blueprint.get("cluster_module_settings", {})
  nodepool_settings = blueprint.get("nodepool_module_settings", {})
  project_id = v.get("project_id") or "<YOUR_PROJECT_ID>"
  zone = v.get("zone") or "<YOUR_ZONE>"
  region = v.get("region") or "<YOUR_REGION>"
  cli_region = blueprint.get("cli_region") or region
  blueprint_name = blueprint.get("blueprint_name") or "my-cluster"
  deployment_name = v.get("deployment_name") or blueprint_name

  result += "Parsed Blueprint Configuration:\n\n"
  result += (
      "# 1. Blueprint vars fragment (merge into 'vars:' in your canonical"
      " blueprint):\n"
  )
  result += dump_yaml({
      "blueprint_name": blueprint_name,
      "vars": v,
  })

  if cluster_settings:
    result += (
        "\n# 2. Cluster module settings patch (apply directly under"
        " modules/scheduler/gke-cluster 'settings:'):\n"
    )
    result += (
        "# Note: Apply these settings directly to the gke-cluster module in"
        " your blueprint.\n# Do NOT place these in global 'vars:', as explicit"
        " module settings in canonical blueprints\n# block global variable"
        " propagation and trigger 'test_deployment_variable_not_used'"
        " validation errors.\n"
    )
    result += dump_yaml({"cluster_module_settings": cluster_settings})

  if nodepool_settings:
    result += (
        "\n# 3. Node pool module settings patch (apply directly under"
        " modules/compute/gke-node-pool 'settings:'):\n"
    )
    result += (
        "# Note: Apply these settings directly to the accelerator gke-node-pool"
        " module in your blueprint.\n# In modern unified blueprints (A3"
        " Mega/High/Ultra, A4), booleans (spot, enable_flex_start)\n# may also"
        " be passed via vars:, but reservation_affinity is an object that must"
        " be configured in YAML.\n# In TPU and other blueprints, reservation_affinity"
        " references $(vars.reservation) so the reservation\n# name is passed"
        " via vars: and gcluster deploy --vars.\n"
    )
    result += dump_yaml({"nodepool_module_settings": nodepool_settings})

  vars_list = [
      f"project_id={project_id}",
      f"deployment_name={deployment_name}",
      f"zone={zone}",
      f"region={cli_region}",
  ]
  if v.get("reservation"):
    vars_list.append(f"reservation={v['reservation']}")
  if v.get("spot") is True:
    vars_list.append("spot=true")
  if v.get("enable_flex_start") is True:
    vars_list.append("enable_flex_start=true")
  vars_str = ",".join(vars_list)
  # Angle brackets are shell redirection operators, so `<YOUR_PROJECT_ID>`
  # pasted into a terminal silently creates a junk file instead of erroring.
  # Render placeholders bare, then wrap in DOUBLE quotes: that neutralises
  # `<`, `>` and spaces while still letting shell variables carried over from
  # the source xpk script (e.g. zone=$ZONE) expand as the user expects.
  vars_str = re.sub(r"<([A-Za-z_][A-Za-z0-9_]*)>", r"\1", vars_str)

  result += "\nInstruction & Deployment Command:\n"
  result += (
      "# Merge the above vars and module settings into your canonical"
      " blueprint (e.g. examples/gke-tpu-v6e/gke-tpu-v6e.yaml) and deploy"
      "\n# (replace any remaining uppercase placeholders first):\n"
  )
  result += f'gcluster deploy BLUEPRINT_FILE.yaml --vars "{vars_str}"\n'
  return result


def format_workload_output(
    cmd_tokens: list[str], warnings: list[str], unmapped_flags: list[str]
) -> str:
  """Formats workload creation command tokens into printable string."""
  result = ""
  for w in warnings:
    result += f"# Note: {w}\n"
  if unmapped_flags:
    flags_str = " ".join(unmapped_flags)
    result += (
        f"# Warning: Unmapped xpk flags were ignored: {flags_str}\n"
        "# The tool is not sure how to translate these unrecognized flag(s) into Cluster Toolkit.\n"
        "# Please verify whether an equivalent setting is needed in your Cluster Toolkit configuration or consult the migration guide.\n"
    )
  result += shlex.join(cmd_tokens)
  return result


def parse_xpk_command(cmd_string: str | None) -> str:
  """Parses XPK command string and returns Cluster Toolkit output."""
  if not cmd_string or not cmd_string.strip():
    return "Error: Incomplete xpk command"
  # Collapse backslash line continuations followed by newlines/whitespace
  cleaned_cmd = re.sub(r"\\\s*\n", " ", cmd_string)
  try:
    tokens = shlex.split(cleaned_cmd)
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
    return f"Error: Subcommand '{subcommand}' is not supported by the parser."


def main(argv: list[str] | None = None) -> None:
  if argv is None:
    argv = sys.argv

  args = argv[1:]
  if not args:
    print(
        "Usage: parse_xpk_to_gcluster.py --xpk_command='xpk workload create ...'"
    )
    sys.exit(1)

  cmd_string = None
  if args[0].startswith("--xpk_command="):
    first_token = args[0].split("=", 1)[1]
    if len(args) > 1:
      cmd_string = shlex.join([first_token] + args[1:])
    else:
      cmd_string = first_token
  elif args[0] == "--xpk_command" and len(args) > 1:
    if len(args) == 2:
      cmd_string = args[1]
    else:
      cmd_string = shlex.join(args[1:])
  elif len(args) == 1:
    cmd_string = args[0]
  else:
    cmd_string = shlex.join(args)

  output = parse_xpk_command(cmd_string)
  print(output)


if __name__ == "__main__":
  main()
