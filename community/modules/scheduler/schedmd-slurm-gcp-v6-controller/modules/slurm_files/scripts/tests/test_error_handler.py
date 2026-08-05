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

import pytest
from error_handler import classify_gcp_error, Action

class TestErrorHandler:
    @pytest.mark.parametrize("error_code, error_msg, expected_action", [
        # Explicit Quota/Capacity failures -> REQUEUE
        ("quotaExceeded", "Quota PREEMPTIBLE_CPUS exceeded", Action.REQUEUE),
        ("ZONE_RESOURCE_POOL_EXHAUSTED", "The zone 'abc' does not have enough resources", Action.REQUEUE),
        ("RESOURCE_EXHAUSTED", "Resource exhausted", Action.REQUEUE),
        ("RATE_LIMIT_EXCEEDED", "rateLimitExceeded", Action.REQUEUE),
        
        # Message fallback match -> REQUEUE
        ("random_code", "we do not have sufficient capacity for this instance type", Action.REQUEUE),

        # Non-capacity errors -> IGNORE
        ("invalidParameter", "Invalid value for field 'resource.networkInterfaces[0].network'", Action.IGNORE),
        ("notFound", "The resource 'projects/.../machineTypes/a3-highgpu-8g' was not found", Action.IGNORE),
        ("permissionDenied", "Required 'compute.instances.create' permission for 'projects/abc'", Action.IGNORE),
    ])
    def test_classify_gcp_error(self, error_code, error_msg, expected_action):
        action, _ = classify_gcp_error(error_code, error_msg)
        assert action == expected_action
