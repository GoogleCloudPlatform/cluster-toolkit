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

"""A cluster's controller answering PING marks an initialising cluster ready.

The controller's C2 daemon reports "ready" once, when it starts. A cluster
redeployed while its controller is already up would otherwise stay
"initialising" forever. Like the other tests here, no Django is imported:
c2.py is loaded by path with its imports stubbed.
"""

import importlib.util
import sys
import types
import unittest
from pathlib import Path
from unittest import mock

CLUSTER_MANAGER = Path(__file__).resolve().parents[1] / "cluster_manager"


def _module(name, **attrs):
    module = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    return module


def _package(name):
    package = _module(name)
    package.__path__ = []
    return package


def _load_c2(cluster_model):
    stubs = {
        "ofe_c2": _package("ofe_c2"),
        "ofe_c2.cluster_manager": _package("ofe_c2.cluster_manager"),
        "ofe_c2.cluster_manager.utils": mock.MagicMock(),
        "ofe_c2.models": _module("ofe_c2.models", Cluster=cluster_model),
        "google": _package("google"),
        "google.api_core": _package("google.api_core"),
        "google.api_core.exceptions": _module(
            "google.api_core.exceptions", AlreadyExists=Exception
        ),
        "google.cloud": _module("google.cloud", pubsub=mock.MagicMock()),
    }
    with mock.patch.dict(sys.modules, stubs):
        spec = importlib.util.spec_from_file_location(
            "ofe_c2.cluster_manager.c2", CLUSTER_MANAGER / "c2.py"
        )
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
    # The PONG handler imports ..models lazily, at call time.
    return module, stubs


class PongMarksReadyTests(unittest.TestCase):

    def setUp(self):
        self.cluster = mock.MagicMock()
        self.c2, self.stubs = _load_c2(self.cluster)

    def _pong(self, source_id):
        with mock.patch.dict(sys.modules, self.stubs):
            return self.c2.c2_pong({"id": "ready-check-7"}, source_id)

    def test_pong_marks_only_an_initialising_cluster_ready(self):
        self.assertTrue(self._pong("cluster_7"))
        self.cluster.objects.filter.assert_called_once_with(pk=7, status="i")
        self.cluster.objects.filter.return_value.update.assert_called_once_with(
            status="r"
        )

    def test_pong_from_unknown_source_changes_nothing(self):
        for source in ("", None, "cluster_x", "workbench_7"):
            with self.subTest(source=source):
                self._pong(source)
        self.cluster.objects.filter.assert_not_called()


class PingClusterTests(unittest.TestCase):

    def setUp(self):
        self.c2, _ = _load_c2(mock.MagicMock())
        self.c2._C2STATE = mock.MagicMock()

    def test_ping_targets_the_cluster_subscription(self):
        self.c2.ping_cluster(7)
        kwargs = self.c2._C2STATE.send_message.call_args.kwargs
        self.assertEqual(kwargs["command"], "PING")
        self.assertEqual(kwargs["target"], "cluster_7")
        self.assertIn("id", kwargs["message"])

    def test_ping_failure_is_not_raised(self):
        self.c2._C2STATE.send_message.side_effect = RuntimeError("pubsub down")
        self.c2.ping_cluster(7)  # must not raise


if __name__ == "__main__":
    unittest.main()
