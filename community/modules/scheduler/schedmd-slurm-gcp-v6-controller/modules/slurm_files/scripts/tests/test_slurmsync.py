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

from unittest import mock

import common  # noqa: F401 - configures the scripts import path for tests
import slurmsync
import util


def test_capacity_circuit_owned_node_is_unchanged(monkeypatch):
    lkp = mock.Mock()
    lkp.node_state.return_value = util.NodeState(
        "DOWN", frozenset({"CLOUD", "POWERED_DOWN"})
    )
    lkp.node_is_gke.return_value = False
    lkp.node_is_fr.return_value = False
    lkp.node_is_dyn.return_value = False
    lkp.node_is_tpu.return_value = False
    monkeypatch.setattr(slurmsync, "lookup", lambda: lkp)
    monkeypatch.setattr(slurmsync.capacity_circuit, "owns_node", lambda node, _: True)

    action = slurmsync.get_node_action("c-n-0")

    assert isinstance(action, slurmsync.NodeActionUnchanged)
    lkp.instance.assert_not_called()


def test_unowned_down_powered_node_keeps_existing_idle_action(monkeypatch):
    lkp = mock.Mock()
    lkp.node_state.return_value = util.NodeState(
        "DOWN", frozenset({"CLOUD", "POWERED_DOWN"})
    )
    lkp.node_is_gke.return_value = False
    lkp.node_is_fr.return_value = False
    lkp.node_is_dyn.return_value = False
    lkp.node_is_tpu.return_value = False
    lkp.instance.return_value = None
    monkeypatch.setattr(slurmsync, "lookup", lambda: lkp)
    monkeypatch.setattr(slurmsync.capacity_circuit, "owns_node", lambda node, _: False)

    action = slurmsync.get_node_action("c-n-0")

    assert isinstance(action, slurmsync.NodeActionIdle)


def test_main_refreshes_lookup_after_reconfigure(monkeypatch):
    old_lookup = mock.Mock(is_controller=False)
    new_lookup = mock.Mock(is_controller=False)
    lookup = mock.Mock(side_effect=[old_lookup, new_lookup])
    monkeypatch.setattr(slurmsync, "lookup", lookup)
    monkeypatch.setattr(slurmsync.util, "should_mount_slurm_bucket", lambda: False)
    reconfigure = mock.Mock()
    monkeypatch.setattr(slurmsync, "reconfigure_slurm", reconfigure)
    install_scripts = mock.Mock()
    monkeypatch.setattr(slurmsync, "install_custom_scripts", install_scripts)

    slurmsync.main()

    reconfigure.assert_called_once_with()
    assert lookup.call_count == 2
    install_scripts.assert_called_once_with(check_hash=True)
