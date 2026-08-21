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
        "gcluster deploy test-cluster.yaml --vars"
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

  def test_missing_machine_types_file_fallback(self):
    with mock.patch.object(
        parse_xpk_to_gcluster.os.path, "exists", return_value=False
    ):
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
    self.assertIn("gs://my-bucket/data;/mnt/data;rw", output)

  def test_parse_workload_create_maps_storage_variable(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16"
        " --storage $STORAGE_URI"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount '$STORAGE_URI;/mnt/storage_uri;rw'", output)

  def test_parse_workload_create_maps_storage_variable_with_subpath(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16"
        " --storage $STORAGE_URI/subfolder"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount '$STORAGE_URI/subfolder;/mnt/subfolder;rw'", output)

  def test_parse_workload_create_maps_storage_with_trailing_slash(self):
    cmd = (
        "xpk workload create --workload job1 --tpu-type v6e-16"
        " --storage gs://my-bucket/dir/"
    )
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("gs://my-bucket/dir/;/mnt/dir;rw", output)

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

  def test_tpu_variable_adds_topology_placeholder_in_workload(self):
    cmd = "xpk workload create --workload job1 --tpu-type $TPU_DEVICE"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--compute-type '$TPU_DEVICE'", output)
    self.assertIn("--topology '<YOUR_TOPOLOGY>'", output)

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

  def test_main_flag_format(self):
    with mock.patch("builtins.print") as mock_print:
      parse_xpk_to_gcluster.main(
          ["parse_xpk_to_gcluster.py", "--xpk_command=xpk workload create --workload job1 --tpu-type v6e-16"]
      )
      mock_print.assert_called_once()
      printed_output = mock_print.call_args[0][0]
      self.assertIn("gcluster job submit --name job1", printed_output)

  def test_main_unquoted_positional_arguments(self):
    with mock.patch("builtins.print") as mock_print:
      parse_xpk_to_gcluster.main(
          ["parse_xpk_to_gcluster.py", "xpk", "workload", "create", "--workload", "job1", "--tpu-type", "v6e-16"]
      )
      mock_print.assert_called_once()
      printed_output = mock_print.call_args[0][0]
      self.assertIn("gcluster job submit --name job1", printed_output)

  def test_main_flag_with_unquoted_args(self):
    with mock.patch("builtins.print") as mock_print:
      parse_xpk_to_gcluster.main(
          ["parse_xpk_to_gcluster.py", "--xpk_command", "xpk", "workload", "create", "--workload", "job1", "--tpu-type", "v6e-16"]
      )
      mock_print.assert_called_once()
      printed_output = mock_print.call_args[0][0]
      self.assertIn("gcluster job submit --name job1", printed_output)

  def test_gpu_variable_omits_tpu_topology_in_cluster(self):
    cmd = "xpk cluster create --cluster c1 --device-type $GPU_DEVICE"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("machine_type: $GPU_DEVICE", output)
    self.assertNotIn("tpu_topology", output)

  def test_single_chip_tpu_resolution(self):
    m_type, top = parse_xpk_to_gcluster.get_machine_type("v6e-1")
    self.assertEqual(m_type, "ct6e-standard-1t")
    self.assertEqual(top, "1x1")

    m_type, top = parse_xpk_to_gcluster.get_machine_type("v5litepod-1")
    self.assertEqual(m_type, "ct5lp-hightpu-1t")
    self.assertEqual(top, "1x1")

  def test_enable_lustre_csi_driver(self):
    cmd = "xpk cluster create --cluster c1 --enable-lustre-csi-driver"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_managed_lustre_csi: true", output)
    self.assertNotIn("enable_parallelstore_csi_driver", output)

  def test_enable_parallelstore_csi_driver(self):
    cmd = "xpk cluster create --cluster c1 --enable-parallelstore-csi-driver"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("enable_parallelstore_csi_driver: true", output)

  def test_storage_variable_with_path_suffix(self):
    cmd = "xpk workload create --workload job1 --tpu-type v6e-16 --storage $STORAGE_URI/data"
    output = parse_xpk_to_gcluster.parse_xpk_command(cmd)
    self.assertIn("--mount '$STORAGE_URI/data;/mnt/data;rw'", output)

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
    self.assertIn("--mount 'gs://$BUCKET_NAME;/mnt/bucket_name;rw'", output)

    cmd2 = "xpk workload create --workload job1 --tpu-type v6e-16 --storage gs://${BUCKET_NAME}"
    output2 = parse_xpk_to_gcluster.parse_xpk_command(cmd2)
    self.assertIn("--mount 'gs://${BUCKET_NAME};/mnt/bucket_name;rw'", output2)


if __name__ == "__main__":
  unittest.main()
