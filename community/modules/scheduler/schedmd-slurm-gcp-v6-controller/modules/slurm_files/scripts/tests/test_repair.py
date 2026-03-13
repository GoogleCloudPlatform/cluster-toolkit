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

import unittest
from unittest.mock import patch, mock_open, MagicMock
import json
import fcntl
from datetime import datetime, timezone
from pathlib import Path
import subprocess

import repair

class RepairScriptTest(unittest.TestCase):

    def setUp(self):
        # Reset the REPAIR_FILE path for each test
        repair.REPAIR_FILE = Path("/slurm/repair_operations.json")

    @patch('repair._get_operations')
    def test_is_node_being_repaired(self, mock_get_operations):
        mock_get_operations.return_value = {
            "node-1": {"status": "REPAIR_IN_PROGRESS"},
            "node-2": {"status": "SUCCESS"}
        }
        self.assertTrue(repair.is_node_being_repaired("node-1"))
        self.assertFalse(repair.is_node_being_repaired("node-2"))
        self.assertFalse(repair.is_node_being_repaired("node-3"))

    @patch('builtins.open', new_callable=mock_open, read_data='{"node-1": {"status": "REPAIR_IN_PROGRESS"}}')
    @patch('pathlib.Path.exists', return_value=True)
    def test_get_operations_success(self, mock_exists, mock_open_file):
        ops = repair._get_operations()
        self.assertEqual(ops, {"node-1": {"status": "REPAIR_IN_PROGRESS"}})
        mock_open_file.assert_called_with(repair.REPAIR_FILE, 'r', encoding='utf-8')

    @patch('builtins.open', new_callable=mock_open, read_data='invalid json')
    @patch('pathlib.Path.exists', return_value=True)
    def test_get_operations_json_decode_error(self, mock_exists, mock_open_file):
        ops = repair._get_operations()
        self.assertEqual(ops, {})

    @patch('pathlib.Path.exists', return_value=False)
    def test_get_operations_file_not_found(self, mock_exists):
        ops = repair._get_operations()
        self.assertEqual(ops, {})

    @patch('builtins.open', new_callable=mock_open)
    @patch('fcntl.lockf')
    def test_write_all_operations(self, mock_lockf, mock_open_file):
        operations = {"node-1": {"status": "SUCCESS"}}
        repair._write_all_operations(operations)
        mock_open_file.assert_called_with(repair.REPAIR_FILE, 'a', encoding='utf-8')
        handle = mock_open_file()
        expected_json_string = json.dumps(operations, indent=4)
        written_data_parts = [call_args[0][0] for call_args in handle.write.call_args_list]
        written_data = ''.join(written_data_parts)
        self.assertEqual(written_data, expected_json_string)
        mock_lockf.assert_any_call(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        mock_lockf.assert_any_call(handle, fcntl.LOCK_UN)

    @patch('repair._get_operations', return_value={})
    @patch('repair._write_all_operations')
    @patch('repair.datetime')
    def test_store_operation(self, mock_datetime, mock_write_all_operations, mock_get_operations):
        mock_now = datetime(2025, 1, 1, tzinfo=timezone.utc)
        mock_datetime.now.return_value = mock_now

        repair.store_operation("node-1", "op-123", "PERFORMANCE")
        
        expected_operations = {
            "node-1": {
                "operation_id": "op-123",
                "reason": "PERFORMANCE",
                "status": "REPAIR_IN_PROGRESS",
                "timestamp": mock_now.isoformat(),
            }
        }
        mock_write_all_operations.assert_called_with(expected_operations)

    @patch('repair.lookup')
    @patch('repair.run')
    def test_call_rr_api_success(self, mock_run, mock_lookup):
        mock_instance = MagicMock()
        mock_instance.zone = "us-central1-a"
        mock_lookup.return_value.instance.return_value = mock_instance
        
        mock_run.return_value = MagicMock(
            stdout='[{"name": "op-123"}]',
            stderr='',
            returncode=0
        )
        
        op_id = repair.call_rr_api("node-1", "XID")
        self.assertEqual(op_id, "op-123")
        mock_run.assert_called_once()

    @patch('repair.lookup')
    def test_call_rr_api_instance_not_found(self, mock_lookup):
        mock_lookup.return_value.instance.return_value = None
        op_id = repair.call_rr_api("node-1", "XID")
        self.assertIsNone(op_id)

    @patch('repair.lookup')
    @patch('repair.run', side_effect=subprocess.CalledProcessError(1, 'cmd'))
    def test_call_rr_api_run_error(self, mock_run, mock_lookup):
        mock_instance = MagicMock()
        mock_instance.zone = "us-central1-a"
        mock_lookup.return_value.instance.return_value = mock_instance
        op_id = repair.call_rr_api("node-1", "XID")
        self.assertIsNone(op_id)

    @patch('repair.run')
    def test_get_operation_status_success(self, mock_run):
        mock_run.return_value.stdout = '[{"status": "DONE"}]'
        status = repair._get_operation_status("op-123")
        self.assertEqual(status, {"status": "DONE"})
        
    @patch('repair.run')
    def test_get_operation_status_empty_list(self, mock_run):
        mock_run.return_value.stdout = '[]'
        status = repair._get_operation_status("op-123")
        self.assertIsNone(status)

    @patch('repair._get_operations')
    @patch('repair._write_all_operations')
    @patch('repair._get_operation_status')
    @patch('repair.lookup')
    @patch('repair.run')
    def test_poll_operations(self, mock_run, mock_lookup, mock_get_op_status, mock_store_ops, mock_get_ops):
        # Setup initial operations data
        mock_get_ops.return_value = {
            "node-1": {"operation_id": "op-1", "status": "REPAIR_IN_PROGRESS"},
            "node-2": {"operation_id": "op-2", "status": "REPAIR_IN_PROGRESS"},
            "node-3": {"status": "SUCCESS"},
            "node-4": {"status": "RECOVERED"}
        }

        # Mock responses for get_operation_status
        mock_get_op_status.side_effect = [
            {"status": "DONE"},
            {"status": "DONE", "error": "Something went wrong"},
        ]

        # Mock instance status for the SUCCESS case
        mock_instance = MagicMock()
        mock_instance.status = "RUNNING"
        mock_lookup.return_value.instance.return_value = mock_instance

        # Run the poll
        repair.poll_operations()

        # Check the stored operations
        final_ops = mock_store_ops.call_args[0][0]
        self.assertEqual(final_ops["node-1"]["status"], "SUCCESS")
        self.assertEqual(final_ops["node-2"]["status"], "FAILURE")
        self.assertEqual(final_ops["node-3"]["status"], "SUCCESS")
        self.assertEqual(final_ops["node-4"]["status"], "RECOVERED")
        
        # Check that scontrol was called correctly
        self.assertIn("update nodename=node-1 state=power_down", mock_run.call_args_list[0][0][0])
        self.assertIn("update nodename=node-2 state=down", mock_run.call_args_list[1][0][0])

if __name__ == '__main__':
    unittest.main()
