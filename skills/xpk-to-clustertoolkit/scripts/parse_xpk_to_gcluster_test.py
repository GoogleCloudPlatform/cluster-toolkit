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

"""Unit tests for parse_xpk_to_gcluster.py."""

import os
import re
import unittest
from unittest import mock

import parse_xpk_to_gcluster


class ParseXpkToGclusterTest(unittest.TestCase):

  def test_is_tpu_hardware(self):
    self.assertTrue(parse_xpk_to_gcluster.is_tpu_hardware("v6e-8", None))
    self.assertTrue(parse_xpk_to_gcluster.is_tpu_hardware("v8e-16", None))
    self.assertTrue(parse_xpk_to_gcluster.is_tpu_hardware(None, "tpu7x-128"))
    self.assertFalse(
        parse_xpk_to_gcluster.is_tpu_hardware("a4-highgpu-8g", "a4-highgpu-8g")
    )
    self.assertFalse(parse_xpk_to_gcluster.is_tpu_hardware("v100-8", None))
    self.assertFalse(parse_xpk_to_gcluster.is_tpu_hardware(None, "v100"))

  def test_is_flag_true(self):
    self.assertTrue(parse_xpk_to_gcluster.is_flag_true("true"))
    self.assertTrue(parse_xpk_to_gcluster.is_flag_true("1"))
    self.assertTrue(parse_xpk_to_gcluster.is_flag_true("yes"))
    self.assertFalse(parse_xpk_to_gcluster.is_flag_true("none"))
    self.assertFalse(parse_xpk_to_gcluster.is_flag_true("false"))
    self.assertFalse(parse_xpk_to_gcluster.is_flag_true("0"))
    self.assertFalse(parse_xpk_to_gcluster.is_flag_true(None))

  def test_parse_workload_create_basic(self):
    cmd = (
        "xpk workload create --workload my-job --tpu-type tpu7x-128"
        " --docker-image gcr.io/my-img --command python3 train.py"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("gcluster job submit", output)
    self.assertIn("--name my-job", output)
    self.assertIn("--compute-type tpu7x-standard-4t", output)
    self.assertIn("--topology 4x4x8", output)
    self.assertIn("--image gcr.io/my-img", output)

  def test_parse_workload_create_pathways(self):
    cmd = (
        "xpk workload create-pathways --workload my-pw --tpu-type tpu7x-128"
        " --headless=true --pathways-gcs-location gs://my-bucket/tmp"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--pathways", output)
    self.assertIn("--pathways-headless", output)
    self.assertIn("--pathways-gcs-location gs://my-bucket/tmp", output)

  def test_parse_workload_create_omits_num_nodes_for_tpu(self):
    cmd = (
        "xpk workload create --workload tpu-job --tpu-type tpu7x-128"
        " --num-nodes 4 --docker-image gcr.io/my-img"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("Omitted --num-nodes", output)
    self.assertNotIn("--num-nodes", output.splitlines()[-1])

  def test_parse_workload_create_unmapped_flags_omitted_from_tokens(self):
    cmd = (
        "xpk workload create --workload my-job --tpu-type tpu7x-128"
        " --unsupported-flag-abc foo"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn(
        "# Warning: Unmapped xpk flags were ignored: --unsupported-flag-abc"
        " foo",
        output,
    )
    self.assertNotIn("--unsupported-flag-abc", output.splitlines()[-1])

  def test_parse_cluster_create_includes_blueprint_name(self):
    cmd = (
        "xpk cluster create --cluster test-cluster --project my-proj --zone"
        " us-central1-a --tpu-type tpu7x-128"
    )
    blueprint, warnings, unmapped = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster",
            "test-cluster",
            "--project",
            "my-proj",
            "--zone",
            "us-central1-a",
            "--tpu-type",
            "tpu7x-128",
        ],
    )
    self.assertEqual(blueprint["blueprint_name"], "test-cluster")
    self.assertIn("vars", blueprint)

    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("blueprint_name: test-cluster", output)
    self.assertIn("vars:", output)
    self.assertIn(
        "gcluster deploy <blueprint_file.yaml> --vars"
        " project_id=my-proj,deployment_name=test-cluster,zone=us-central1-a,region=us-central1",
        output,
    )
    self.assertNotIn("gcluster create", output)

  def test_authorized_networks_maps_to_authorized_cidr(self):
    cmd = "xpk cluster create --cluster c1 --authorized-networks 10.0.0.0/8"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("authorized_cidr: 10.0.0.0/8", output)

  def test_on_demand_does_not_set_spot(self):
    cmd = "xpk cluster create --cluster c1 --on-demand"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertNotIn("spot: true", output)

  def test_parse_cluster_create_explicit_boolean(self):
    cmd = (
        "xpk cluster create --cluster c1 --private=true --enable-pathways=true"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_private_endpoint: true", output)
    self.assertIn("enable_pathways_for_tpus: true", output)
    self.assertIn("n4-standard-64", output)

  def test_parse_cluster_create_unmapped_flags_warning(self):
    cmd = "xpk cluster create --cluster c1 --custom-unsupported-flag foo"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("Warning: Unmapped xpk flags were ignored", output)

  def test_tensorboard_warning(self):
    cmd = (
        "xpk cluster create --cluster c1 --create-vertex-tensorboard"
        " --tensorboard-name my-tb"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn(
        "Cluster Toolkit does not support Vertex Tensorboard creation", output
    )

  def test_unknown_device_type_fallback(self):
    m_type, top = parse_xpk_to_gcluster.get_machine_type("unknown-device")
    self.assertEqual(m_type, "unknown-device")
    self.assertEqual(top, "N/A")

  def test_parse_workload_create_maps_storage_to_mount(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16"
        " --storage gs://my-bucket/data"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount", output)
    self.assertIn("gs://my-bucket/data;/mnt/data;ro", output)
    self.assertIn("references an xpk Storage object", output)

  def test_parse_workload_create_maps_storage_variable(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16"
        " --storage $STORAGE_URI"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount '$STORAGE_URI;/mnt/storage_uri;ro'", output)
    self.assertIn("references an xpk Storage object", output)

  def test_parse_workload_create_maps_storage_variable_with_subpath(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16"
        " --storage $STORAGE_URI/subfolder"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount '$STORAGE_URI/subfolder;/mnt/subfolder;ro'", output)
    self.assertIn("references an xpk Storage object", output)

  def test_parse_workload_create_maps_storage_with_trailing_slash(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16"
        " --storage gs://my-bucket/"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("gs://my-bucket/;/mnt/my-bucket;ro", output)
    self.assertIn("references an xpk Storage object", output)

  def test_invalid_command(self):
    output = parse_xpk_to_gcluster.parse_xpk_command("kubectl get pods")
    self.assertIn("Error: Not an xpk command", output)

  def test_syntax_error_unclosed_quotes(self):
    output = parse_xpk_to_gcluster.parse_xpk_command(
        'xpk workload create --workload "unclosed'
    )
    self.assertIn("Error parsing command:", output)

  def test_workload_create_command_flag_preserved(self):
    cmd = (
        'xpk workload create --workload job1 --tpu-type v6e-16 --command'
        ' "python3 train.py --lr=0.01"'
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--command 'python3 train.py --lr=0.01'", output)

  def test_parse_cluster_create_with_zone_variable(self):
    cmd = "xpk cluster create --cluster c1 --zone=$ZONE"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("zone=$ZONE", output)
    self.assertIn("region=${ZONE%-*}", output)

    cmd_braced = "xpk cluster create --cluster c1 --zone=${MY_ZONE}"
    output_braced = parse_xpk_to_gcluster.parse_xpk_command(cmd_braced)
    self.assertIn("zone=${MY_ZONE}", output_braced)
    self.assertIn("region=${MY_ZONE%-*}", output_braced)

  def test_main_single_quoted_argument(self):
    with mock.patch("builtins.print") as mock_print:
      parse_xpk_to_gcluster.main(
          ["parse_xpk_to_gcluster.py", "xpk workload create --workload job1 --tpu-type v6e-16"]
      )
      mock_print.assert_called_once()
      printed_output = mock_print.call_args[0][0]
      self.assertIn("gcluster job submit --name job1", printed_output)

  def test_gpu_variable_omits_topology_in_workload(self):
    cmd = "xpk workload create --workload job1 --device-type $GPU_DEVICE"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--compute-type '$GPU_DEVICE'", output)
    self.assertNotIn("--topology", output)

  def test_tpu_variable_in_workload(self):
    cmd = "xpk workload create --workload job1 --tpu-type $TPU_DEVICE"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--compute-type '$TPU_DEVICE'", output)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", output)
    self.assertIn("TPU workloads require an explicit --topology", output)

  def test_tpu7x_variable_in_workload(self):
    cmd = "xpk workload create --workload job1 --tpu-type tpu7x-${N}"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--compute-type tpu7x-standard-4t", output)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", output)
    self.assertIn("TPU 7x requires an explicit --topology", output)

  def test_tpu7x_unknown_sizes_require_placeholder(self):
    cmd = "xpk workload create --workload job1 --tpu-type tpu7x-384"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--compute-type tpu7x-standard-4t", output)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", output)
    self.assertNotIn("4x4x4", output)
    self.assertIn("Could not determine TPU 7x topology for 'tpu7x-384'", output)

    cmd2 = "xpk workload create --workload job1 --tpu-type tpu7x-1536"
    output2 = parse_xpk_to_gcluster.parse_xpk_command(cmd2)
    self.assertIn("--compute-type tpu7x-standard-4t", output2)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", output2)
    self.assertNotIn("4x4x4", output2)
    self.assertIn("Could not determine TPU 7x topology for 'tpu7x-1536'", output2)

  def test_custom_named_variable_in_tpu_type(self):
    cmd = "xpk workload create --workload job1 --tpu-type $MY_ACCELERATOR --num-nodes 4"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--compute-type '$MY_ACCELERATOR'", output)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", output)
    self.assertIn("Omitted --num-nodes", output)

    cluster_cmd = "xpk cluster create --cluster c1 --tpu-type $MY_ACCELERATOR"
    cluster_output = parse_xpk_to_gcluster.parse_xpk_command(cluster_cmd)
    self.assertIn("machine_type: $MY_ACCELERATOR", cluster_output)
    self.assertIn("tpu_topology: <YOUR_TOPOLOGY>", cluster_output)

  def test_cores_vs_chips_topology_mapping(self):
    # v4-N: N is cores. Chips = N / 2.
    m_type, top = parse_xpk_to_gcluster.get_machine_type("v4-8")
    self.assertEqual(m_type, "ct4p-hightpu-4t")
    self.assertEqual(top, "2x2x1")  # 4 chips

    m_type, top = parse_xpk_to_gcluster.get_machine_type("v4-128")
    self.assertEqual(m_type, "ct4p-hightpu-4t")
    self.assertEqual(top, "4x4x4")  # 64 chips

    # v5p-N: N is cores. Chips = N / 2.
    m_type, top = parse_xpk_to_gcluster.get_machine_type("v5p-8")
    self.assertEqual(m_type, "ct5p-hightpu-4t")
    self.assertEqual(top, "2x2x1")  # 4 chips

    m_type, top = parse_xpk_to_gcluster.get_machine_type("v5p-128")
    self.assertEqual(m_type, "ct5p-hightpu-4t")
    self.assertEqual(top, "4x4x4")  # 64 chips

    # v6e-N: N is chips.
    m_type, top = parse_xpk_to_gcluster.get_machine_type("v6e-16")
    self.assertEqual(m_type, "ct6e-standard-4t")
    self.assertEqual(top, "4x4")  # 16 chips

    # tpu7x-N: N is chips.
    m_type, top = parse_xpk_to_gcluster.get_machine_type("tpu7x-128")
    self.assertEqual(m_type, "tpu7x-standard-4t")
    self.assertEqual(top, "4x4x8")  # 128 chips

  def test_workload_create_accelerator_resolution(self):
    # v6e-16 resolves to ct6e-standard-4t with explicit topology 4x4
    out_v6e = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j1 --tpu-type v6e-16")
    self.assertIn("--compute-type ct6e-standard-4t --topology 4x4", out_v6e)

    # v5e family resolves to ct5lp-hightpu-*t with explicit topology
    out_v5e_16 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j2 --tpu-type v5e-16")
    self.assertIn("--compute-type ct5lp-hightpu-4t --topology 4x4", out_v5e_16)

    out_v5e_8 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j3 --tpu-type v5e-8")
    self.assertIn("--compute-type ct5lp-hightpu-8t --topology 2x4", out_v5e_8)

    out_v5e_4 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j4 --tpu-type v5e-4")
    self.assertIn("--compute-type ct5lp-hightpu-4t --topology 2x2", out_v5e_4)

    out_v5e_1 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j5 --tpu-type v5e-1")
    self.assertIn("--compute-type ct5lp-hightpu-1t --topology 1x1", out_v5e_1)

    # Explicit dimensions for v6e
    out_v6e_1x1 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j6 --tpu-type v6e-1x1")
    self.assertIn("--compute-type ct6e-standard-1t --topology 1x1", out_v6e_1x1)

    out_v6e_2x4 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j7 --tpu-type v6e-2x4")
    self.assertIn("--compute-type ct6e-standard-8t --topology 2x4", out_v6e_2x4)

    out_v6e_4x4 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j8 --tpu-type v6e-4x4")
    self.assertIn("--compute-type ct6e-standard-4t --topology 4x4", out_v6e_4x4)

    # v4 and v5p multi-slice sizes resolve to machine type + topology
    out_v4 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j9 --tpu-type v4-128")
    self.assertIn("--compute-type ct4p-hightpu-4t --topology 4x4x4", out_v4)

    out_v5p = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j10 --tpu-type v5p-128")
    self.assertIn("--compute-type ct5p-hightpu-4t --topology 4x4x4", out_v5p)

    # tpu7x explicitly maps to tpu7x-standard-4t with --topology
    out_7x = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j11 --tpu-type tpu7x-128")
    self.assertIn("--compute-type tpu7x-standard-4t --topology 4x4x8", out_7x)

    # Literal keys in Go's AcceleratorShorthandMap pass through directly
    out_v6e4 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j12 --tpu-type v6e-4")
    self.assertIn("--compute-type v6e-4", out_v6e4)

    out_v48 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j13 --tpu-type v4-8")
    self.assertIn("--compute-type v4-8", out_v48)

    out_l4 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j14 --device-type l4-8")
    self.assertIn("--compute-type l4-8", out_l4)

    out_h100 = parse_xpk_to_gcluster.parse_xpk_command("xpk workload create --workload j15 --device-type h100-80gb-8")
    self.assertIn("--compute-type h100-80gb-8", out_h100)

  def test_pathways_gcs_location_warning(self):
    cmd = "xpk workload create-pathways --workload pw-job --tpu-type v6e-16"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--pathways-gcs-location is required by gcluster", output)

  def test_blueprint_variable_names_and_types(self):
    cmd = (
        "xpk cluster create --cluster my-c --project my-p --zone us-central1-a"
        " --tpu-type v6e-16 --num-slices 2 --enable-gcsfuse-csi-driver"
        " --enable-filestore-csi-driver --enable-parallelstore-csi-driver"
        " --enable-managed-disk-csi-driver --system-node-pool-node-count 3"
        " --host-maintenance-interval PERIODIC --enable-ml-diagnostics"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_gcsfuse_csi: true", output)
    self.assertIn("enable_filestore_csi: true", output)
    self.assertIn("enable_parallelstore_csi: true", output)
    self.assertIn("enable_persistent_disk_csi: true", output)
    self.assertIn("system_node_pool_node_count:\n    total_min_nodes: 3\n    total_max_nodes: 3", output)
    self.assertIn("host_maintenance_interval: PERIODIC", output)
    self.assertIn("enable_ml_diagnostics: true", output)
    self.assertIn("num_slices: 2", output)

  def test_enable_lustre_csi_driver(self):
    cmd = "xpk cluster create --cluster c1 --enable-lustre-csi-driver"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_managed_lustre_csi: true", output)
    self.assertNotIn("enable_parallelstore_csi", output)

  def test_enable_parallelstore_csi_driver(self):
    cmd = "xpk cluster create --cluster c1 --enable-parallelstore-csi-driver"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_parallelstore_csi: true", output)

  def test_storage_variable_with_path_suffix(self):
    cmd = "xpk workload create --workload job1 --tpu-type v6e-16 --storage $STORAGE_URI/data"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount '$STORAGE_URI/data;/mnt/data;ro'", output)

  def test_additional_workload_flags(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16 --timeout 24h"
        " --queue my-queue --gke-namespace custom-ns --skip-prereqs"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--timeout 24h", output)
    self.assertIn("--queue my-queue", output)
    self.assertIn("--gke-namespace custom-ns", output)
    self.assertIn("--skip-prereqs", output)

  def test_prepended_environment_variables(self):
    cmd = "PROJECT_ID=my-project ZONE=us-central1-a xpk workload create --workload job1 --tpu-type v6e-16"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("gcluster job submit --name job1", output)

  def test_storage_variable_with_bucket_prefix(self):
    cmd = "xpk workload create --workload job1 --tpu-type v6e-16 --storage gs://$BUCKET_NAME"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount 'gs://$BUCKET_NAME;/mnt/bucket_name;ro'", output)

    cmd2 = "xpk workload create --workload job1 --tpu-type v6e-16 --storage gs://${BUCKET_NAME}"
    output2 = parse_xpk_to_gcluster.parse_xpk_command(cmd2)
    self.assertIn("--mount 'gs://${BUCKET_NAME};/mnt/bucket_name;ro'", output2)

  def test_cluster_create_mtc(self):
    cmd = "xpk cluster create --cluster my-cluster --project my-project --zone us-central1-a --tpu-type v6e-16 --num-slices 2 --enable-mtc --mtc-ramdisk-size 50Gi"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_multi_tier_checkpointing: true", output)
    self.assertIn("In Cluster Toolkit, MTC ramdisk directory and storage buckets are configured", output)

  def test_cluster_create_zone_parameter_expansion(self):
    cmd = "xpk cluster create --cluster my-cluster --project my-project --zone ${ZONE:-us-central1-a} --tpu-type v6e-16"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("region=${ZONE%-*}", output)
    self.assertIn("region: <YOUR_REGION>", output)

  def test_main_cli_variations(self):
    with mock.patch("builtins.print") as mock_print:
      parse_xpk_to_gcluster.main([
          "parse_xpk_to_gcluster.py",
          "--xpk_command=xpk",
          "workload",
          "create",
          "--workload",
          "job1",
          "--tpu-type",
          "v6e-16",
      ])
      mock_print.assert_called_once()
      printed = mock_print.call_args[0][0]
      self.assertIn("gcluster job submit --name job1", printed)

    with mock.patch("builtins.print") as mock_print:
      parse_xpk_to_gcluster.main([
          "parse_xpk_to_gcluster.py",
          "--xpk_command",
          "xpk",
          "workload",
          "create",
          "--workload",
          "job2",
          "--tpu-type",
          "v6e-16",
      ])
      mock_print.assert_called_once()
      printed = mock_print.call_args[0][0]
      self.assertIn("gcluster job submit --name job2", printed)

  def test_hardware_go_static_tables_golden(self):
    """Verifies that embedded Python static tables match pkg/config/hardware.go byte-for-byte."""
    cur_dir = os.path.dirname(os.path.abspath(__file__))
    hw_path = None
    candidate = cur_dir
    while candidate != os.path.dirname(candidate):
      check_path = os.path.join(candidate, "pkg", "config", "hardware.go")
      if os.path.exists(check_path):
        hw_path = check_path
        break
      candidate = os.path.dirname(candidate)

    if not hw_path or not os.path.exists(hw_path):
      self.skipTest(f"hardware.go not found relative to {cur_dir}")

    with open(hw_path, "r", encoding="utf-8") as f:
      content = f.read()

    # 1. Parse AcceleratorShorthandMap
    shorthand_match = re.search(
        r"var AcceleratorShorthandMap = map\[string\]string\{([^}]+)\}", content
    )
    self.assertIsNotNone(
        shorthand_match,
        "Failed to locate AcceleratorShorthandMap in hardware.go",
    )
    go_shorthands = {}
    for line in shorthand_match.group(1).strip().splitlines():
      line = line.strip()
      if not line or line.startswith("//"):
        continue
      m = re.match(r"\"([^\"]+)\":\s*\"([^\"]+)\",?", line)
      if m:
        go_shorthands[m.group(1)] = m.group(2)

    self.assertEqual(
        parse_xpk_to_gcluster.STATIC_ACCELERATOR_SHORTHAND_MAP,
        go_shorthands,
        "STATIC_ACCELERATOR_SHORTHAND_MAP does not match"
        " pkg/config/hardware.go AcceleratorShorthandMap",
    )

    # 2. Parse common3DTopologies
    match_3d = re.search(
        r"var common3DTopologies = map\[int\]string\{([^}]+)\}", content
    )
    self.assertIsNotNone(
        match_3d, "Failed to locate common3DTopologies in hardware.go"
    )
    go_3d = {}
    for line in match_3d.group(1).strip().splitlines():
      line = line.strip()
      if not line or line.startswith("//"):
        continue
      m = re.match(r"(\d+):\s*\"([^\"]+)\",?", line)
      if m:
        go_3d[int(m.group(1))] = m.group(2)

    self.assertEqual(
        parse_xpk_to_gcluster.COMMON_3D_TOPOLOGIES,
        go_3d,
        "COMMON_3D_TOPOLOGIES does not match pkg/config/hardware.go"
        " common3DTopologies",
    )

    # 3. Parse allowed2DTopologies
    match_2d = re.search(
        r"var allowed2DTopologies = map\[int\]string\{([^}]+)\}", content
    )
    self.assertIsNotNone(
        match_2d, "Failed to locate allowed2DTopologies in hardware.go"
    )
    go_2d = {}
    for line in match_2d.group(1).strip().splitlines():
      line = line.strip()
      if not line or line.startswith("//"):
        continue
      m = re.match(r"(\d+):\s*\"([^\"]+)\",?", line)
      if m:
        go_2d[int(m.group(1))] = m.group(2)

    self.assertEqual(
        parse_xpk_to_gcluster.ALLOWED_2D_TOPOLOGIES,
        go_2d,
        "ALLOWED_2D_TOPOLOGIES does not match pkg/config/hardware.go"
        " allowed2DTopologies",
    )

  def test_cluster_module_settings_separation(self):
    cmd = (
        "xpk cluster create --cluster my-cluster --project my-project --zone"
        " us-central1-a --tpu-type v6e-16 --num-slices 2"
        " --default-pool-cpu-machine-type n2-standard-16"
        " --default-pool-cpu-num-nodes 4 --private --enable-gcsfuse-csi-driver"
        " --enable-ml-diagnostics"
    )
    blueprint, warnings, unmapped = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster",
            "my-cluster",
            "--project",
            "my-project",
            "--zone",
            "us-central1-a",
            "--tpu-type",
            "v6e-16",
            "--num-slices",
            "2",
            "--default-pool-cpu-machine-type",
            "n2-standard-16",
            "--default-pool-cpu-num-nodes",
            "4",
            "--private",
            "--enable-gcsfuse-csi-driver",
            "--enable-ml-diagnostics",
        ],
    )
    vars_block = blueprint["vars"]
    module_settings = blueprint["cluster_module_settings"]

    # Module settings must NOT be in top-level vars
    self.assertNotIn("system_node_pool_machine_type", vars_block)
    self.assertNotIn("system_node_pool_node_count", vars_block)
    self.assertNotIn("enable_private_endpoint", vars_block)
    self.assertNotIn("enable_gcsfuse_csi", vars_block)
    self.assertNotIn("enable_ml_diagnostics", vars_block)

    # Module settings must be present under cluster_module_settings
    self.assertEqual(
        module_settings.get("system_node_pool_machine_type"), "n2-standard-16"
    )
    self.assertEqual(
        module_settings.get("system_node_pool_node_count"),
        {"total_min_nodes": 4, "total_max_nodes": 4},
    )
    self.assertTrue(module_settings.get("enable_private_endpoint"))
    self.assertTrue(module_settings.get("enable_gcsfuse_csi"))
    self.assertTrue(module_settings.get("enable_ml_diagnostics"))

    # Top-level vars must contain canonical blueprint inputs
    self.assertEqual(vars_block.get("deployment_name"), "my-cluster")
    self.assertEqual(vars_block.get("project_id"), "my-project")
    self.assertEqual(vars_block.get("zone"), "us-central1-a")
    self.assertEqual(vars_block.get("region"), "us-central1")
    self.assertEqual(vars_block.get("machine_type"), "ct6e-standard-4t")
    self.assertEqual(vars_block.get("tpu_topology"), "4x4")
    self.assertEqual(vars_block.get("num_slices"), 2)

    # Output text formatting check
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("# 1. Blueprint vars fragment", output)
    self.assertIn("# 2. Cluster module settings patch", output)

  def test_deprecated_tpu_v2_v3(self):
    cmd_v2 = "xpk workload create --workload j1 --tpu-type v2-8"
    out_v2 = parse_xpk_to_gcluster.parse_xpk_command(cmd_v2)
    self.assertIn("ERROR: TPU v2/v3 are deprecated", out_v2)

    cmd_v3 = "xpk workload create --workload j2 --tpu-type v3-8"
    out_v3 = parse_xpk_to_gcluster.parse_xpk_command(cmd_v3)
    self.assertIn("ERROR: TPU v2/v3 are deprecated", out_v3)

  def test_bare_tpu7x_workload_and_cluster(self):
    cmd_wl = "xpk workload create --workload j1 --tpu-type tpu7x --command 'python3 train.py'"
    out_wl = parse_xpk_to_gcluster.parse_xpk_command(cmd_wl)
    self.assertIn("--compute-type tpu7x-standard-4t", out_wl)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", out_wl)
    self.assertIn("Could not determine TPU 7x topology for 'tpu7x'", out_wl)

    cmd_cl = "xpk cluster create --cluster c1 --tpu-type tpu7x"
    out_cl = parse_xpk_to_gcluster.parse_xpk_command(cmd_cl)
    self.assertIn("machine_type: tpu7x-standard-4t", out_cl)
    self.assertIn("tpu_topology: <YOUR_TOPOLOGY>", out_cl)

  def test_tpu7x_trailing_dash(self):
    cmd = "xpk workload create --workload j1 --tpu-type tpu7x- --command 'python3 train.py'"
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--compute-type tpu7x-standard-4t", out)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", out)

  def test_gpu_via_tpu_type_preserves_num_nodes(self):
    cmd = "xpk workload create --workload j1 --tpu-type h100-mega-80gb-8 --num-nodes 2 --command 'python3 train.py'"
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--num-nodes 2", out)
    self.assertIn("--compute-type h100-mega-80gb-8", out)
    self.assertNotIn("Omitted --num-nodes", out)

  def test_unknown_accelerator_shorthand_warning(self):
    cmd = "xpk workload create --workload j1 --device-type v100-8 --command 'python3 train.py'"
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("is not a known accelerator shorthand in Cluster Toolkit", out)
    self.assertIn("--compute-type v100-8", out)

  def test_deprecated_tpu_in_cluster_create_does_not_inject_error_to_machine_type(self):
    cmd = "xpk cluster create --cluster c1 --tpu-type v2-8"
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("ERROR: TPU v2/v3 are deprecated", out)
    self.assertNotIn("machine_type: 'ERROR", out)
    self.assertNotIn("machine_type: ERROR", out)

  def test_storage_mount_destination_deduplication(self):
    cmd = (
        "xpk workload create --workload j1 --tpu-type v6e-16 --command 'python3 train.py'"
        " --storage gs://bucket-a/data --storage gs://bucket-b/data"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("gs://bucket-a/data;/mnt/data;ro", out)
    self.assertIn("gs://bucket-b/data;/mnt/data_2;ro", out)

  def test_unsupported_storage_scheme_warning(self):
    cmd = "xpk workload create --workload j1 --tpu-type v6e-16 --command 'python3 train.py' --storage s3://bucket/data"
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("uses an unsupported scheme", out)

  def test_cluster_create_with_shell_variables_and_reservation(self):
    cmd = "xpk cluster create --cluster $CLUSTER_NAME --project $PROJECT_ID --zone $ZONE --reservation $RESERVATION_NAME --tpu-type v6e-16"
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("blueprint_name: my-cluster", out)
    self.assertIn("deployment_name: $CLUSTER_NAME", out)
    self.assertIn("reservation: $RESERVATION_NAME", out)
    self.assertIn("reservation=$RESERVATION_NAME", out)
    self.assertIn("deployment_name=$CLUSTER_NAME", out)
    self.assertIn("region=${ZONE%-*}", out)

  def test_colocated_python_sidecar_image(self):
    cmd = (
        'xpk workload create-pathways --workload pw-mtc --cluster my-cluster'
        ' --project my-project --zone us-central1-a --tpu-type v6e-16'
        ' --docker-image gcr.io/my-project/head:v1'
        ' --colocated-python-sidecar-image gcr.io/my-project/sidecar:v1'
        ' --pathways-gcs-location gs://my-bucket/tmp --command "python3 train.py"'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn(
        "--pathways-colocated-python-sidecar-image gcr.io/my-project/sidecar:v1",
        out,
    )
    self.assertNotIn(
        "--pathways-colocated-python-sidecar-image is only supported for", out
    )

  def test_colocated_python_sidecar_image_non_pathways_warning(self):
    cmd = (
        'xpk workload create --workload std-job --cluster my-cluster'
        ' --project my-project --zone us-central1-a --tpu-type v6e-16'
        ' --docker-image gcr.io/my-project/img:v1'
        ' --colocated-python-sidecar-image gcr.io/my-project/sidecar:v1'
        ' --command "python3 train.py"'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn(
        "--pathways-colocated-python-sidecar-image is only supported for"
        " Pathways workloads",
        out,
    )

  def test_base_docker_image_without_script_dir_maps_to_image(self):
    cmd = (
        'xpk workload create --workload server-job --cluster my-cluster'
        ' --project my-project --zone us-central1-a --tpu-type v6e-16'
        ' --base-docker-image gcr.io/my-project/img:v1'
        ' --command "python3 server.py"'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--image gcr.io/my-project/img:v1", out)
    self.assertNotIn("--base-image", out)
    self.assertNotIn("--build-context", out)

  def test_base_docker_image_with_script_dir_maps_to_crane(self):
    cmd = (
        'xpk workload create --workload server-job --cluster my-cluster'
        ' --project my-project --zone us-central1-a --tpu-type v6e-16'
        ' --base-docker-image gcr.io/my-project/base:v1 --script-dir .'
        ' --command "python3 server.py"'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--base-image gcr.io/my-project/base:v1", out)
    self.assertIn("--build-context .", out)

  def test_docker_image_with_script_dir_emits_image_and_warns(self):
    cmd = (
        'xpk workload create --workload server-job --cluster my-cluster'
        ' --project my-project --zone us-central1-a --tpu-type v6e-16'
        ' --docker-image gcr.io/my-project/base:v1 --script-dir /src'
        ' --command "python3 server.py"'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--image gcr.io/my-project/base:v1", out)
    self.assertNotIn("--base-image", out)
    self.assertNotIn("--build-context", out)
    self.assertIn(
        "cannot be combined with `--base-docker-image` or `--script-dir`", out
    )

  def test_workload_name_length_warnings(self):
    # Pathways limit > 22
    cmd_pw = (
        'xpk workload create-pathways --workload long-pathways-job-name-123'
        ' --cluster my-cluster --project my-project --zone us-central1-a'
        ' --tpu-type v6e-16 --docker-image img:v1'
        ' --pathways-gcs-location gs://b/t --command "echo hi"'
    )
    out_pw = parse_xpk_to_gcluster.parse_xpk_command(cmd_pw)
    self.assertIn("exceeds 22 characters", out_pw)
    self.assertIn("63-byte coordinator label limit", out_pw)

    # Standard limit > 28
    cmd_std = (
        'xpk workload create --workload this-is-a-very-long-standard-workload-name'
        ' --cluster my-cluster --project my-project --zone us-central1-a'
        ' --tpu-type v6e-16 --docker-image img:v1 --command "echo hi"'
    )
    out_std = parse_xpk_to_gcluster.parse_xpk_command(cmd_std)
    self.assertIn("exceeds 28 characters", out_std)

    # Shell variable does not trigger false warning
    cmd_var = (
        'xpk workload create-pathways --workload "${RUN_NAME}"'
        ' --cluster my-cluster --project my-project --zone us-central1-a'
        ' --tpu-type v6e-16 --docker-image img:v1'
        ' --pathways-gcs-location gs://b/t --command "echo hi"'
    )
    out_var = parse_xpk_to_gcluster.parse_xpk_command(cmd_var)
    self.assertNotIn("exceeds 22 characters", out_var)

  def test_restart_on_exit_codes_validation_and_deduplication(self):
    cmd = (
        'xpk workload create --workload retry-job --cluster my-cluster'
        ' --project my-project --zone us-central1-a --tpu-type v6e-16'
        ' --docker-image img:v1 --command "echo hi"'
        ' --restart-on-exit-codes 1,2,1,255,0,300'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--restart-on-exit-codes 1,2,255", out)
    self.assertIn("Invalid exit code '0' in --restart-on-exit-codes", out)
    self.assertIn("Invalid exit code '300' in --restart-on-exit-codes", out)

  def test_cluster_dynamic_consumption_warnings(self):
    cmd_res = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --device-type h100-mega-80gb-8 --num-nodes 8'
        ' --reservation my-reservation'
    )
    out_res = parse_xpk_to_gcluster.parse_xpk_command(cmd_res)
    self.assertIn("SPECIFIC_RESERVATION", out_res)
    self.assertIn("my-reservation", out_res)

    cmd_od = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --device-type h100-mega-80gb-8 --num-nodes 8'
        ' --on-demand'
    )
    out_od = parse_xpk_to_gcluster.parse_xpk_command(cmd_od)
    self.assertIn("NO_RESERVATION", out_od)

  def test_reservation_affinity_schema_unified_gpu(self):
    cmd_res = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --device-type h100-mega-80gb-8 --num-nodes 8'
        ' --reservation my-reservation'
    )
    blueprint, warnings, unmapped = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "my-cluster",
            "--project", "my-project",
            "--zone", "us-central1-a",
            "--device-type", "h100-mega-80gb-8",
            "--num-nodes", "8",
            "--reservation", "my-reservation",
        ],
    )
    # Unified GPU blueprints declare reservation_affinity directly in vars:
    self.assertIn("reservation_affinity", blueprint["vars"])
    self.assertNotIn("reservation", blueprint["vars"])
    res_aff = blueprint["vars"]["reservation_affinity"]
    self.assertEqual(res_aff["consume_reservation_type"], "SPECIFIC_RESERVATION")
    self.assertEqual(res_aff["specific_reservations"], [{"name": "my-reservation"}])
    self.assertNotIn("reservation_affinity", blueprint["nodepool_module_settings"])

    out = parse_xpk_to_gcluster.parse_xpk_command(cmd_res)
    self.assertNotIn("compute.googleapis.com/reservation-name", out)
    self.assertNotIn("$(vars.reservation)", out)
    self.assertIn("- name: my-reservation", out)
    self.assertNotIn("reservation=my-reservation", out)

  def test_reservation_affinity_schema_tpu_and_other_architectures(self):
    cmd_res = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --tpu-type v6e-16'
        ' --reservation my-reservation'
    )
    blueprint, warnings, unmapped = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "my-cluster",
            "--project", "my-project",
            "--zone", "us-central1-a",
            "--tpu-type", "v6e-16",
            "--reservation", "my-reservation",
        ],
    )
    # TPU and other blueprints declare reservation in vars: and reference it in nodepool settings
    self.assertEqual(blueprint["vars"]["reservation"], "my-reservation")
    self.assertNotIn("reservation_affinity", blueprint["vars"])
    nodepool_settings = blueprint["nodepool_module_settings"]
    self.assertIn("reservation_affinity", nodepool_settings)
    res_aff = nodepool_settings["reservation_affinity"]
    self.assertEqual(res_aff["consume_reservation_type"], "SPECIFIC_RESERVATION")
    self.assertEqual(res_aff["specific_reservations"], [{"name": "$(vars.reservation)"}])

    out = parse_xpk_to_gcluster.parse_xpk_command(cmd_res)
    self.assertNotIn("compute.googleapis.com/reservation-name", out)
    self.assertIn("reservation: my-reservation", out)
    self.assertIn("- name: $(vars.reservation)", out)
    self.assertNotIn("- name: my-reservation\n", out)
    self.assertIn("reservation=my-reservation", out)

  def test_nodepool_module_settings_separation_for_spot(self):
    cmd_spot = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --tpu-type v6e-16'
        ' --spot'
    )
    blueprint, warnings, unmapped = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "my-cluster",
            "--project", "my-project",
            "--zone", "us-central1-a",
            "--tpu-type", "v6e-16",
            "--spot",
        ],
    )
    self.assertNotIn("spot", blueprint["cluster_module_settings"])
    self.assertNotIn("spot", blueprint["vars"])
    self.assertTrue(blueprint["nodepool_module_settings"].get("spot"))
    res_aff = blueprint["nodepool_module_settings"].get("reservation_affinity")
    self.assertEqual(res_aff, {"consume_reservation_type": "NO_RESERVATION", "specific_reservations": []})

    out = parse_xpk_to_gcluster.parse_xpk_command(cmd_spot)
    self.assertIn("nodepool_module_settings:", out)
    self.assertIn("spot: true", out)

  def test_nodepool_module_settings_separation_for_flex(self):
    cmd_flex = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --tpu-type v6e-16'
        ' --flex'
    )
    blueprint, warnings, unmapped = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "my-cluster",
            "--project", "my-project",
            "--zone", "us-central1-a",
            "--tpu-type", "v6e-16",
            "--flex",
        ],
    )
    self.assertNotIn("enable_flex_start", blueprint["cluster_module_settings"])
    self.assertNotIn("enable_flex_start", blueprint["vars"])
    self.assertNotIn("static_node_count", blueprint["vars"])
    np = blueprint["nodepool_module_settings"]
    self.assertTrue(np.get("enable_flex_start"))
    self.assertFalse(np.get("auto_repair"))
    self.assertEqual(np.get("static_node_count"), "$(null)")
    self.assertEqual(np.get("reservation_affinity"), {"consume_reservation_type": "NO_RESERVATION", "specific_reservations": []})

    out = parse_xpk_to_gcluster.parse_xpk_command(cmd_flex)
    self.assertIn("enable_flex_start: true", out)
    self.assertIn("auto_repair: false", out)
    self.assertIn("static_node_count: $(null)", out)

  def test_unified_gpu_spot_consumption(self):
    cmd = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --device-type h100-mega-80gb-8 --num-nodes 4'
        ' --spot'
    )
    blueprint, warnings, _ = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "my-cluster",
            "--project", "my-project",
            "--zone", "us-central1-a",
            "--device-type", "h100-mega-80gb-8",
            "--num-nodes", "4",
            "--spot",
        ],
    )
    self.assertTrue(blueprint["vars"].get("spot"))
    self.assertNotIn("spot", blueprint["nodepool_module_settings"])
    self.assertNotIn("reservation_affinity", blueprint["nodepool_module_settings"])
    self.assertEqual(blueprint["nodepool_module_settings"], {})

    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("spot: true", out)
    self.assertIn("spot=true", out)
    self.assertNotIn("nodepool_module_settings:", out)

  def test_unified_gpu_flex_consumption(self):
    cmd = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --device-type h100-mega-80gb-8 --num-nodes 4'
        ' --flex'
    )
    blueprint, warnings, _ = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "my-cluster",
            "--project", "my-project",
            "--zone", "us-central1-a",
            "--device-type", "h100-mega-80gb-8",
            "--num-nodes", "4",
            "--flex",
        ],
    )
    self.assertTrue(blueprint["vars"].get("enable_flex_start"))
    self.assertEqual(blueprint["vars"].get("static_node_count"), "$(null)")
    self.assertNotIn("enable_flex_start", blueprint["nodepool_module_settings"])
    self.assertNotIn("auto_repair", blueprint["nodepool_module_settings"])
    self.assertNotIn("static_node_count", blueprint["nodepool_module_settings"])
    self.assertNotIn("reservation_affinity", blueprint["nodepool_module_settings"])
    self.assertEqual(blueprint["nodepool_module_settings"], {})

    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_flex_start: true", out)
    self.assertNotIn("auto_repair:", out)
    self.assertNotIn("reservation_affinity:", out)
    self.assertNotIn("nodepool_module_settings:", out)
    self.assertIn("enable_flex_start=true", out)

  def test_cluster_create_unknown_accelerator_shorthand_warning(self):
    cmd = (
        'xpk cluster create --cluster c1 --project p1 --zone us-central1-a'
        ' --device-type v100-8 --num-nodes 1'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("is not a known accelerator shorthand", out)

  def test_consumption_mutual_exclusivity_warning(self):
    cmd_conflict = (
        'xpk cluster create --cluster my-cluster --project my-project'
        ' --zone us-central1-a --device-type h100-mega-80gb-8 --num-nodes 4'
        ' --spot --flex'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd_conflict)
    self.assertIn("Conflicting consumption options specified", out)

  def test_workload_create_job_level_consumption(self):
    cmd_spot = (
        'xpk workload create --workload w1 --cluster c1 --zone us-central1-a'
        ' --device-type h100-mega-80gb-8 --num-nodes 4 --spot'
        ' --docker-image gcr.io/img:v1 --command "echo hi"'
    )
    out_spot = parse_xpk_to_gcluster.parse_xpk_command(cmd_spot)
    self.assertIn("--gke-nap-provisioning spot", out_spot)

    cmd_od = (
        'xpk workload create --workload w1 --cluster c1 --zone us-central1-a'
        ' --device-type h100-mega-80gb-8 --num-nodes 4 --on-demand'
        ' --docker-image gcr.io/img:v1 --command "echo hi"'
    )
    out_od = parse_xpk_to_gcluster.parse_xpk_command(cmd_od)
    self.assertIn("--gke-nap-provisioning on-demand", out_od)

    cmd_res = (
        'xpk workload create --workload w1 --cluster c1 --zone us-central1-a'
        ' --device-type h100-mega-80gb-8 --num-nodes 4 --reservation my-res'
        ' --docker-image gcr.io/img:v1 --command "echo hi"'
    )
    out_res = parse_xpk_to_gcluster.parse_xpk_command(cmd_res)
    self.assertIn("--gke-nap-provisioning reservation", out_res)
    self.assertIn("--gke-nap-reservation my-res", out_res)

  def test_workload_create_deploy_stacktrace_sidecar(self):
    cmd = (
        'xpk workload create --workload w1 --cluster c1 --zone us-central1-a'
        ' --device-type h100-mega-80gb-8 --num-nodes 4 --deploy-stacktrace-sidecar'
        ' --docker-image gcr.io/img:v1 --command "echo hi"'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--verbose", out)
    self.assertNotIn("Unmapped", out)

  def test_workload_create_registered_submit_flags_passthrough(self):
    cmd = (
        'xpk workload create-pathways --workload pw1 --cluster c1 --zone us-central1-a'
        ' --device-type v6e-16 --pathways-gcs-location gs://b/t'
        ' --docker-image gcr.io/img:v1 --cpu-affinity numa'
        ' --gke-custom-templates-path /tmp/tmpl --pathways-head-np head-np'
        ' --pathways-worker-image gcr.io/w:v1 --platform linux/arm64'
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--cpu-affinity numa", out)
    self.assertIn("--gke-custom-templates-path /tmp/tmpl", out)
    self.assertIn("--pathways-head-np head-np", out)
    self.assertIn("--pathways-worker-image gcr.io/w:v1", out)
    self.assertIn("--platform linux/arm64", out)
    self.assertNotIn("Unmapped", out)

  def test_docs_and_references_reservation_affinity_schema(self):
    """Asserts that no documentation or reference file contains the invalid key/values schema."""
    script_dir = os.path.dirname(os.path.abspath(__file__))
    skill_root = os.path.dirname(script_dir)
    target_str = "compute.googleapis.com/" + "reservation-name"
    for root, _, files in os.walk(skill_root):
      for file in files:
        if file.endswith(("_test.py", ".pyc")):
          continue
        if file.endswith((".md", ".py", ".yaml", ".yml")):
          fpath = os.path.join(root, file)
          with open(fpath, "r", encoding="utf-8") as f:
            content = f.read()
            self.assertNotIn(
                target_str,
                content,
                f"File {fpath} contains forbidden internal string {target_str}",
            )
            if "specific_reservations" in content:
              self.assertNotRegex(
                  content,
                  r"specific_reservations:\s*\[\s*\{[^}]*key:",
                  f"File {fpath} contains invalid reservation_affinity schema with 'key:'",
              )

  def test_references_examples_md_consistency(self):
    """Guarantees that commands documented in references/examples.md match parser outputs."""
    # Recipe 1A: MaxText SFT (McJAX Mode)
    cmd_sft_mcjax = (
        'xpk workload create --cluster="${GKE_CLUSTER}"'
        ' --project="${PROJECT_ID}" --zone="${LOCATION}"'
        ' --workload="${RUN_NAME}" --tpu-type=v5p-128 --num-slices=1'
        ' --docker-image="${DOCKER_IMAGE}"'
        ' --command="python3 -m maxtext.trainers.post_train.sft.train_sft run_name=${RUN_NAME} base_output_directory=${BASE_OUTPUT_DIRECTORY} model_name=${MODEL} steps=${STEPS}"'
    )
    out_sft_mcjax = parse_xpk_to_gcluster.parse_xpk_command(cmd_sft_mcjax)
    self.assertIn("--compute-type ct5p-hightpu-4t", out_sft_mcjax)
    self.assertIn("--topology 4x4x4", out_sft_mcjax)
    self.assertIn("--num-slices 1", out_sft_mcjax)
    self.assertNotIn("--pathways", out_sft_mcjax)

    # Recipe 1B: MaxText SFT (Pathways Mode)
    cmd_sft_pw = (
        'xpk workload create-pathways --cluster="${GKE_CLUSTER}"'
        ' --project="${PROJECT_ID}" --zone="${LOCATION}"'
        ' --workload="${RUN_NAME}" --tpu-type=v5p-128 --num-slices=1'
        ' --docker-image="${DOCKER_IMAGE}"'
        ' --pathways-gcs-location="${BASE_OUTPUT_DIRECTORY}"'
        ' --command="python3 -m maxtext.trainers.post_train.sft.train_sft run_name=${RUN_NAME} base_output_directory=${BASE_OUTPUT_DIRECTORY} model_name=${MODEL} steps=${STEPS} enable_single_controller=True"'
    )
    out_sft_pw = parse_xpk_to_gcluster.parse_xpk_command(cmd_sft_pw)
    self.assertIn("--pathways", out_sft_pw)
    self.assertIn("--compute-type ct5p-hightpu-4t", out_sft_pw)
    self.assertIn("--topology 4x4x4", out_sft_pw)
    self.assertIn("--pathways-gcs-location '${BASE_OUTPUT_DIRECTORY}'", out_sft_pw)

    # Recipe 2: MaxText Elastic Training with Pathways
    cmd_elastic = (
        'xpk workload create-pathways --cluster="${GKE_CLUSTER}"'
        ' --project="${PROJECT_ID}" --zone="${LOCATION}"'
        ' --workload="${RUN_NAME}" --tpu-type=v5litepod-16 --num-slices=3'
        ' --docker-image="${DOCKER_IMAGE}" --elastic-slices=1'
        ' --max-slice-restarts=10'
        ' --pathways-gcs-location="${BASE_OUTPUT_DIRECTORY}"'
        ' --command="python3 -m maxtext.trainers.pre_train.train src/maxtext/configs/base.yml base_output_directory=${BASE_OUTPUT_DIRECTORY} run_name=${RUN_NAME} num_slices=3 enable_single_controller=True"'
    )
    out_elastic = parse_xpk_to_gcluster.parse_xpk_command(cmd_elastic)
    self.assertIn("--pathways", out_elastic)
    self.assertIn("--compute-type ct5lp-hightpu-4t", out_elastic)
    self.assertIn("--topology 4x4", out_elastic)
    self.assertIn("--num-slices 3", out_elastic)
    self.assertIn("--pathways-elastic-slices 1", out_elastic)
    self.assertIn("--pathways-max-slice-restarts 10", out_elastic)

    # Recipe 3: MaxText Multi-Tier Checkpointing (MTC)
    cmd_mtc = (
        'xpk workload create-pathways --cluster="${CLUSTER_NAME}"'
        ' --project="${PROJECT_ID}" --zone="${LOCATION}"'
        ' --workload="${WORKLOAD_NAME}" --tpu-type=v6e-256 --num-slices=1'
        ' --docker-image="${MAXTEXT_IMAGE}"'
        ' --colocated-python-sidecar-image="${COLOCATED_PYTHON_IMAGE}"'
        ' --ramdisk-directory="${RAMDISK_DIRECTORY}" --mtc-enabled'
        ' --pathways-gcs-location="${OUTPUT_PATH}"'
        ' --command="python3 -m maxtext.trainers.pre_train.train src/maxtext/configs/base.yml run_name=${WORKLOAD_NAME} base_output_directory=${OUTPUT_PATH} enable_multi_tier_checkpointing=True local_checkpoint_directory=${RAMDISK_DIRECTORY}"'
    )
    out_mtc = parse_xpk_to_gcluster.parse_xpk_command(cmd_mtc)
    self.assertIn("--pathways", out_mtc)
    self.assertIn("--compute-type ct6e-standard-4t", out_mtc)
    self.assertIn("--topology 16x16", out_mtc)
    self.assertIn("--gke-mtc-enabled", out_mtc)
    self.assertIn("--gke-mtc-ramdisk-dir '${RAMDISK_DIRECTORY}'", out_mtc)
    self.assertIn(
        "--pathways-colocated-python-sidecar-image '${COLOCATED_PYTHON_IMAGE}'",
        out_mtc,
    )

    # Recipe 4: MaxText RL (tpu7x)
    cmd_pw = (
        'xpk workload create-pathways --cluster="${CLUSTER_NAME}"'
        ' --project="${PROJECT_ID}" --zone="${ZONE}" --priority=medium'
        ' --max-restarts=0 --tpu-type=tpu7x-128 --num-slices=1'
        ' --docker-image="${DOCKER_IMAGE}" --workload="${WORKLOAD_NAME}"'
        ' --custom-pathways-proxy-server-args="${XLA_FLAGS}"'
        ' --command="${MAXTEXT_COMMAND}"'
        ' --pathways-gcs-location="gs://<YOUR_PATHWAYS_STATE_BUCKET>/tmp"'
    )
    out_pw = parse_xpk_to_gcluster.parse_xpk_command(cmd_pw)
    self.assertIn("--compute-type tpu7x-standard-4t", out_pw)
    self.assertIn("--topology 4x4x8", out_pw)
    self.assertIn("--pathways-gcs-location 'gs://<YOUR_PATHWAYS_STATE_BUCKET>/tmp'", out_pw)

    # Recipe 5: Multi-Slice TPU v6e
    cmd_v6e = (
        'xpk workload create --workload="v6e-pretrain" --cluster="ml-cluster"'
        ' --project="my-project" --zone="us-central1-a" --tpu-type="v6e-256"'
        ' --num-slices=4 --docker-image="gcr.io/my-project/pretrain:latest"'
        ' --command="python3 train.py --batch-size=1024"'
    )
    out_v6e = parse_xpk_to_gcluster.parse_xpk_command(cmd_v6e)
    self.assertIn("--compute-type ct6e-standard-4t", out_v6e)
    self.assertIn("--topology 16x16", out_v6e)
    self.assertIn("--num-slices 4", out_v6e)
    self.assertNotIn("v6e-256", out_v6e)

    # Recipe 6: NVIDIA H100 GPU
    cmd_gpu = (
        'xpk workload create --workload="h100-finetune" --cluster="gpu-cluster"'
        ' --project="my-project" --zone="us-east5-a"'
        ' --device-type="h100-mega-80gb-8" --num-nodes=8'
        ' --docker-image="gcr.io/my-project/finetune:v1"'
        ' --command="torchrun --nproc_per_node=8 train.py"'
    )
    out_gpu = parse_xpk_to_gcluster.parse_xpk_command(cmd_gpu)
    self.assertIn("--compute-type h100-mega-80gb-8", out_gpu)
    self.assertIn("--num-nodes 8", out_gpu)
    self.assertNotIn("--topology", out_gpu)

  def test_cluster_tpu_without_pathways_explicit_false(self):
    cmd = (
        "xpk cluster create --cluster tpu-c --project p1 --zone us-central1-a"
        " --tpu-type v6e-16"
    )
    blueprint, warnings, _ = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "tpu-c",
            "--project", "p1",
            "--zone", "us-central1-a",
            "--tpu-type", "v6e-16",
        ],
    )
    self.assertIn("enable_pathways_for_tpus", blueprint["vars"])
    self.assertFalse(blueprint["vars"]["enable_pathways_for_tpus"])
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_pathways_for_tpus: false", out)
    self.assertTrue(
        any(
            "enable_pathways_for_tpus: false emitted explicitly" in w
            for w in warnings
        )
    )

  def test_cluster_gpu_without_pathways_omits_pathways_var(self):
    cmd = (
        "xpk cluster create --cluster gpu-c --project p1 --zone us-central1-a"
        " --device-type h100-mega-80gb-8 --num-nodes 4"
    )
    blueprint, warnings, _ = parse_xpk_to_gcluster.parse_cluster_create(
        is_pathways=False,
        unknown=[
            "--cluster", "gpu-c",
            "--project", "p1",
            "--zone", "us-central1-a",
            "--device-type", "h100-mega-80gb-8",
            "--num-nodes", "4",
        ],
    )
    self.assertNotIn("enable_pathways_for_tpus", blueprint["vars"])
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertNotIn("enable_pathways_for_tpus", out)

  def test_workload_create_image_flag_precedence_and_conflicts(self):
    # Case 1: --docker-image alone emits --image
    cmd1 = (
        "xpk workload create --workload w1 --cluster c1 --docker-image img:v1"
        " --command 'echo 1' --device-type h100-mega-80gb-8 --num-nodes 1"
    )
    out1 = parse_xpk_to_gcluster.parse_xpk_command(cmd1)
    self.assertIn("--image img:v1", out1)
    self.assertNotIn("--base-image", out1)
    self.assertNotIn("--build-context", out1)

    # Case 2: --base-docker-image + --script-dir emits --base-image + --build-context
    cmd2 = (
        "xpk workload create --workload w2 --cluster c1 --base-docker-image"
        " base:v1 --script-dir ./src --command 'echo 2' --device-type"
        " h100-mega-80gb-8 --num-nodes 1"
    )
    out2 = parse_xpk_to_gcluster.parse_xpk_command(cmd2)
    self.assertIn("--base-image base:v1", out2)
    self.assertIn("--build-context ./src", out2)
    self.assertNotIn("--image", out2)

    # Case 3: Conflicting combination --docker-image + --base-docker-image + --script-dir
    cmd3 = (
        "xpk workload create --workload w3 --cluster c1 --docker-image img:v2"
        " --base-docker-image base:v1 --script-dir ./src --command 'echo 3'"
        " --device-type h100-mega-80gb-8 --num-nodes 1"
    )
    out3 = parse_xpk_to_gcluster.parse_xpk_command(cmd3)
    # Must emit --image img:v2 per xpk precedence; MUST NOT duplicate or emit base-image/build-context
    self.assertIn("--image img:v2", out3)
    self.assertNotIn("--base-image", out3)
    self.assertNotIn("--build-context", out3)
    self.assertIn(
        "cannot be combined with `--base-docker-image` or `--script-dir`", out3
    )

    # Case 4: Conflicting combination --docker-image + --script-dir (no base image)
    cmd4 = (
        "xpk workload create --workload w4 --cluster c1 --docker-image img:v3"
        " --script-dir ./src --command 'echo 4' --device-type"
        " h100-mega-80gb-8 --num-nodes 1"
    )
    out4 = parse_xpk_to_gcluster.parse_xpk_command(cmd4)
    self.assertIn("--image img:v3", out4)
    self.assertNotIn("--build-context", out4)
    self.assertIn(
        "cannot be combined with `--base-docker-image` or `--script-dir`", out4
    )

    # Case 5: --script-dir alone without --base-docker-image or --docker-image (N3)
    cmd5 = (
        "xpk workload create --workload w5 --cluster c1 --script-dir ./src"
        " --command 'echo 5' --device-type h100-mega-80gb-8 --num-nodes 1"
    )
    out5 = parse_xpk_to_gcluster.parse_xpk_command(cmd5)
    self.assertIn("--base-image python:3.10", out5)
    self.assertIn("--build-context ./src", out5)
    self.assertIn("defaults to `python:3.10`", out5)

  def test_flex_start_preserves_sizing_intent_warning(self):
    cmd = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --device-type h100-mega-80gb-8 --num-nodes 16 --flex"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("autoscaling_total_max_nodes: 16", out)

  def test_fetch_toolkit_examples_script_integrity(self):
    script_path = os.path.join(
        os.path.dirname(__file__), "fetch_toolkit_examples.sh"
    )
    self.assertTrue(os.path.isfile(script_path))
    self.assertTrue(os.access(script_path, os.X_OK))
    with open(script_path, "r", encoding="utf-8") as f:
      content = f.read()
    self.assertIn("set -euo pipefail", content)
    self.assertIn("modules/compute/gke-node-pool", content)
    self.assertIn("${CLUSTER_TOOLKIT_BRANCH:-main}", content)
    self.assertIn('touch "$TIMESTAMP_FILE"', content)


  def test_docker_image_pull_secret_mapping(self):
    cmd_xpk = (
        "xpk workload create --workload w1 --cluster c1 --docker-image img:v1"
        " --command 'echo 1' --device-type h100-mega-80gb-8 --num-nodes 1"
        " --docker-image-pull-secret my-secret"
    )
    out_xpk = parse_xpk_to_gcluster.parse_xpk_command(cmd_xpk)
    self.assertIn("--image-pull-secret my-secret", out_xpk)
    self.assertNotIn("Unmapped", out_xpk)

    # Verify native passthrough also still works
    cmd_native = (
        "xpk workload create --workload w2 --cluster c1 --docker-image img:v1"
        " --command 'echo 2' --device-type h100-mega-80gb-8 --num-nodes 1"
        " --image-pull-secret my-secret"
    )
    out_native = parse_xpk_to_gcluster.parse_xpk_command(cmd_native)
    self.assertIn("--image-pull-secret my-secret", out_native)
    self.assertNotIn("Unmapped", out_native)

  def test_env_file_warning(self):
    cmd = (
        "xpk workload create --workload w1 --cluster c1 --docker-image img:v1"
        " --command 'echo 1' --device-type h100-mega-80gb-8 --num-nodes 1"
        " --env-file /tmp/env.txt"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("`--env-file` is not directly supported by `gcluster job submit`", out)
    self.assertIn("Expand the file's contents into repeated `--env KEY=VALUE` flags", out)
    self.assertNotIn("--env-file", out.splitlines()[-1])

  def test_tpu7x_dynamic_slicing_vars_emission(self):
    cmd_sub = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type tpu7x-4x4x4 --sub-slicing"
    )
    out_sub = parse_xpk_to_gcluster.parse_xpk_command(cmd_sub)
    self.assertIn("enable_dynamic_slicing_for_tpus: true", out_sub)
    self.assertIn("1.35.0-gke.274500", out_sub)

    cmd_super = (
        "xpk cluster create --cluster c2 --project p1 --zone us-central1-a"
        " --tpu-type tpu7x-4x4x4 --super-slicing"
    )
    out_super = parse_xpk_to_gcluster.parse_xpk_command(cmd_super)
    self.assertIn("enable_dynamic_slicing_for_tpus: true", out_super)
    self.assertIn("1.35.0-gke.274500", out_super)

  def test_non_7x_tpu_dynamic_slicing_cluster_settings_fallback(self):
    cmd = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type v6e-16 --super-slicing"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertNotIn("enable_dynamic_slicing_for_tpus:", out)
    self.assertIn("enable_slice_controller: true", out)
    self.assertIn("does not declare enable_dynamic_slicing_for_tpus", out)

  def test_num_cubes_warning(self):
    cmd = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type tpu7x-4x4x4 --super-slicing --num-cubes 2"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn(
        "--num-cubes has no direct Cluster Toolkit flag; express the cube count through tpu_topology instead (1 cube = 64 chips / 4x4x4 mesh; total chips = 2 * 64).",
        out,
    )

  def test_dynamic_slicing_tpu7x_invariants(self):
    cmd = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type tpu7x-4x4x4 --super-slicing"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_dynamic_slicing_for_tpus: true", out)
    self.assertIn("accelerator_topology_mode: PROVISION_ONLY", out)
    self.assertIn("requires a specific compute reservation (`--reservation`)", out)

    cmd_with_res = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type tpu7x-4x4x4 --super-slicing --reservation my-res"
    )
    out_with_res = parse_xpk_to_gcluster.parse_xpk_command(cmd_with_res)
    self.assertIn("enable_dynamic_slicing_for_tpus: true", out_with_res)
    self.assertIn("accelerator_topology_mode: PROVISION_ONLY", out_with_res)
    self.assertNotIn("requires a specific compute reservation (`--reservation`)", out_with_res)

  def test_gke_version_mappings(self):
    cmd_prefix = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type v6e-16 --gke-version 1.31."
    )
    out_prefix = parse_xpk_to_gcluster.parse_xpk_command(cmd_prefix)
    self.assertIn("version_prefix: 1.31.", out_prefix)
    self.assertNotIn("min_master_version:", out_prefix)

    cmd_min = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type v6e-16 --gke-version 1.31.1-gke.100"
    )
    out_min = parse_xpk_to_gcluster.parse_xpk_command(cmd_min)
    self.assertIn("min_master_version: 1.31.1-gke.100", out_min)
    self.assertNotIn("version_prefix:", out_min)

  def test_host_maintenance_interval_in_nodepool_settings(self):
    cmd = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type v6e-16 --host-maintenance-interval PERIODIC"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("host_maintenance_interval: PERIODIC", out)
    lines = out.split("\n")
    nodepool_idx = next(i for i, line in enumerate(lines) if "nodepool_module_settings:" in line)
    maint_idx = next(i for i, line in enumerate(lines) if "host_maintenance_interval: PERIODIC" in line)
    self.assertGreater(maint_idx, nodepool_idx)

  def test_multi_cidr_authorized_networks(self):
    cmd = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type v6e-16 --authorized-networks 10.0.0.0/8,192.168.1.0/24"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("master_authorized_networks:", out)
    self.assertIn("cidr_block: 10.0.0.0/8", out)
    self.assertIn("cidr_block: 192.168.1.0/24", out)
    self.assertIn("display_name: access-net-1", out)
    self.assertIn("display_name: access-net-2", out)

  def test_mtc_and_ml_diagnostics_prerequisites(self):
    cmd = (
        "xpk cluster create --cluster c1 --project p1 --zone us-central1-a"
        " --tpu-type v6e-16 --enable-mtc --enable-ml-diagnostics"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_multi_tier_checkpointing: true", out)
    self.assertIn("enable_gcsfuse_csi: true", out)
    self.assertIn("enable_ml_diagnostics: true", out)
    self.assertIn("configure_workload_identity_sa: true", out)

  def test_pathways_flags_isolation_on_standard_workload(self):
    cmd = (
        "xpk workload create --cluster c1 --workload w1 --tpu-type v6e-16"
        " --colocated-python-sidecar-image gcr.io/sidecar:latest"
        " --pathways-server-env FOO=BAR"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    cmd_line = [line for line in out.splitlines() if not line.startswith("#")][-1]
    self.assertNotIn("--pathways-colocated-python-sidecar-image", cmd_line)
    self.assertNotIn("--pathways-server-env", cmd_line)
    self.assertIn("only supported for Pathways workloads (--pathways)", out)

  def test_workload_nap_mutual_exclusivity(self):
    cmd = (
        "xpk workload create --cluster c1 --workload w1 --tpu-type v6e-16"
        " --spot --reservation my-res"
    )
    out = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    cmd_line = [line for line in out.splitlines() if not line.startswith("#")][-1]
    self.assertIn("Conflicting workload consumption options specified", out)
    self.assertEqual(cmd_line.count("--gke-nap-provisioning"), 1)
    self.assertNotIn("--gke-nap-reservation", cmd_line)

  def test_region_and_location_flags(self):
    cmd_workload_reg = (
        "xpk workload create --cluster c1 --workload w1 --region us-central1 --tpu-type v6e-16"
    )
    out_workload_reg = parse_xpk_to_gcluster.parse_xpk_command(cmd_workload_reg)
    self.assertIn("--location us-central1", out_workload_reg)

    cmd_workload_loc = (
        "xpk workload create --cluster c1 --workload w1 --location us-east4 --tpu-type v6e-16"
    )
    out_workload_loc = parse_xpk_to_gcluster.parse_xpk_command(cmd_workload_loc)
    self.assertIn("--location us-east4", out_workload_loc)

    cmd_cluster_reg = (
        "xpk cluster create --cluster c1 --project p1 --region europe-west4 --tpu-type v6e-16"
    )
    out_cluster_reg = parse_xpk_to_gcluster.parse_xpk_command(cmd_cluster_reg)
    self.assertIn("region: europe-west4", out_cluster_reg)

  def test_defensive_null_handling(self):
    self.assertEqual(parse_xpk_to_gcluster.get_machine_type(None), ("UNKNOWN", "N/A"))
    self.assertEqual(parse_xpk_to_gcluster.get_machine_type(""), ("UNKNOWN", "N/A"))
    self.assertIn("Incomplete", parse_xpk_to_gcluster.parse_xpk_command(None))
    self.assertIn("Incomplete", parse_xpk_to_gcluster.parse_xpk_command(""))
    self.assertIn("Incomplete", parse_xpk_to_gcluster.parse_xpk_command("   "))

  def test_dump_yaml_recursive_fallback(self):
    saved_yaml = parse_xpk_to_gcluster.yaml
    try:
      parse_xpk_to_gcluster.yaml = None
      test_data = {
          "scalar_str": "val",
          "scalar_bool": True,
          "scalar_null": None,
          "scalar_int": 42,
          "nested_dict": {
              "inner_k": "inner_v",
              "inner_bool": False,
          },
          "nested_list": [
              {"k1": "v1", "k2": None},
              {"k3": True},
          ],
      }
      out = parse_xpk_to_gcluster.dump_yaml(test_data)
      self.assertIn("scalar_str: val", out)
      self.assertIn("scalar_bool: true", out)
      self.assertIn("scalar_null: null", out)
      self.assertIn("scalar_int: 42", out)
      self.assertIn("nested_dict:\n  inner_k: inner_v", out)
      self.assertIn("  inner_bool: false", out)
      self.assertIn("- k1: v1", out)
      self.assertIn("  k2: null", out)
      self.assertIn("- k3: true", out)
    finally:
      parse_xpk_to_gcluster.yaml = saved_yaml


if __name__ == "__main__":
  unittest.main()
