#!/usr/bin/env python3

# Copyright 2024 Google LLC
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

import json
import unittest
from unittest.mock import MagicMock, mock_open, patch

import repair


class RepairTest(unittest.TestCase):
    def setUp(self):
        repair.OPERATIONS_FILE = "/tmp/test_operations.json"

    def test_is_node_being_repaired(self):
        operations = [{"node_name": "test-node-1"}]
        with patch("repair.get_operations", return_value=operations):
            self.assertTrue(repair.is_node_being_repaired("test-node-1"))
            self.assertFalse(repair.is_node_being_repaired("test-node-2"))

    def test_get_operations(self):
        with patch("os.path.exists", return_value=False):
            self.assertEqual(repair.get_operations(), [])

        m = mock_open(read_data='[{"node_name": "test-node"}]')
        with patch("builtins.open", m):
            operations = repair.get_operations()
            self.assertEqual(len(operations), 1)
            self.assertEqual(operations[0]["node_name"], "test-node")

    def test_store_operations(self):
        m = mock_open()
        with patch("builtins.open", m):
            repair.store_operations([{"node_name": "test-node"}])
        m.assert_called_once_with("/tmp/test_operations.json", "w")
        handle = m()
        handle.write.assert_called_once_with('[{"node_name": "test-node"}]')

    @patch("repair.store_operations")
    @patch("repair.get_operations", return_value=[])
    def test_store_operation(self, mock_get, mock_store):
        repair.store_operation({"node_name": "new-node"})
        mock_store.assert_called_once_with([{"node_name": "new-node"}])

    @patch("subprocess.run")
    @patch("repair.store_operation")
    def test_call_rr_api(self, mock_store, mock_run):
        mock_run.return_value.stdout = '{"name": "op1", "project": "p", "zone": "z"}'
        op = repair.call_rr_api("test-node")
        self.assertEqual(op["node_name"], "test-node")
        mock_store.assert_called_once()

    @patch("subprocess.run")
    def test_get_operation_status(self, mock_run):
        mock_run.return_value.stdout = '{"status": "DONE"}'
        status = repair.get_operation_status("op1", "zone1", "proj1")
        self.assertEqual(status["status"], "DONE")

    @patch("subprocess.run")
    @patch("repair.store_operations")
    @patch(
        "repair.get_operations",
        return_value=[
            {
                "name": "op1",
                "node_name": "node1",
                "project": "p",
                "zone": "z",
            }
        ],
    )
    @patch(
        "repair.get_operation_status", return_value={"status": "DONE"}
    )
    def test_poll_operations_done(
        self, mock_status, mock_get, mock_store, mock_run
    ):
        repair.poll_operations()
        mock_status.assert_called_once()
        mock_run.assert_called_with(
            "scontrol update nodename=node1 state=IDLE", shell=True
        )
        mock_store.assert_called_with([])

    @patch("subprocess.run")
    @patch("repair.store_operations")
    @patch(
        "repair.get_operations",
        return_value=[
            {
                "name": "op1",
                "node_name": "node1",
                "project": "p",
                "zone": "z",
            }
        ],
    )
    @patch(
        "repair.get_operation_status", return_value={"status": "RUNNING"}
    )
    def test_poll_operations_running(
        self, mock_status, mock_get, mock_store, mock_run
    ):
        repair.poll_operations()
        mock_status.assert_called_once()
        mock_run.assert_not_called()
        mock_store.assert_called_with(
            [{"name": "op1", "node_name": "node1", "project": "p", "zone": "z"}]
        )


if __name__ == "__main__":
    unittest.main()
