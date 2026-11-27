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

"""The Slurm controller hostname a workbench mounts from"""

import inspect
from types import SimpleNamespace

from django.test import SimpleTestCase

from ghpcfe.cluster_manager import workbenchinfo
from ghpcfe.cluster_manager.workbenchinfo import slurm_controller_hostname


class SlurmControllerHostnameTests(SimpleTestCase):
    """Matches how schedmd-slurm-gcp-v6-controller names the controller"""

    def test_default_name_is_first_ten_letters_and_digits_of_cloud_id(self):
        cluster = SimpleNamespace(cloud_id="mycluster-1a2b3c4d")
        self.assertEqual(
            slurm_controller_hostname(cluster), "mycluster1-controller"
        )

    def test_short_name_keeps_hex_suffix_digits(self):
        cluster = SimpleNamespace(cloud_id="hpc-0f1e2d3c")
        self.assertEqual(
            slurm_controller_hostname(cluster), "hpc0f1e2d3-controller"
        )

    def test_leading_non_letters_and_upper_case_are_removed(self):
        cluster = SimpleNamespace(cloud_id="9-Lab-1a2b3c4d")
        self.assertEqual(
            slurm_controller_hostname(cluster), "lab1a2b3c4-controller"
        )

    def test_explicit_slurm_cluster_name_wins(self):
        cluster = SimpleNamespace(
            cloud_id="mycluster-1a2b3c4d", slurm_cluster_name="mycluster1a2b3c4d"
        )
        self.assertEqual(
            slurm_controller_hostname(cluster), "mycluster1a2b3c4d-controller"
        )

    def test_empty_slurm_cluster_name_falls_back_to_default(self):
        cluster = SimpleNamespace(
            cloud_id="mycluster-1a2b3c4d", slurm_cluster_name=""
        )
        self.assertEqual(
            slurm_controller_hostname(cluster), "mycluster1-controller"
        )

    def test_startup_script_no_longer_uses_old_hostname(self):
        source = inspect.getsource(workbenchinfo)
        self.assertNotIn("slurm-{cid}-controller", source)
        self.assertEqual(source.count("mount {controller}:"), 4)
