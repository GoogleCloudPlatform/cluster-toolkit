# Copyright 2026 Google LLC
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

import shlex
import subprocess

import pytest
from mock import Mock, patch
from common import TstCfg  # needed to import util
import util
import setup


@pytest.mark.parametrize("data_type,altered", [("text", True), ("mediumtext", False), ("", False)])
def test_widen_event_cluster_nodes(data_type, altered):
    lkp = util.Lookup(TstCfg(slurm_cluster_name="m22"))
    with patch.object(setup, "run", return_value=Mock(stdout=f"{data_type}\n")) as run:
        setup.widen_event_cluster_nodes(lkp)
    sql = [shlex.split(c.args[0])[-1] for c in run.call_args_list]
    assert "table_name='m22_event_table'" in sql[0]
    assert len(sql) == (2 if altered else 1)
    if altered:
        assert sql[1].startswith("alter table `m22_event_table` modify `cluster_nodes` mediumtext")


def test_widen_event_cluster_nodes_failure_is_logged():
    lkp = util.Lookup(TstCfg(slurm_cluster_name="m22"))
    err = subprocess.CalledProcessError(1, "mysql")
    with patch.object(setup, "run", side_effect=[Mock(stdout="text\n"), err]), \
            patch.object(setup.log, "exception") as log_exc:
        setup.widen_event_cluster_nodes(lkp)
    log_exc.assert_called_once()
