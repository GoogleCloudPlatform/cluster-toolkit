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

import enum

class Action(enum.Enum):
    REQUEUE = "REQUEUE"
    IGNORE = "IGNORE"

def classify_gcp_error(error_code: str, error_message: str) -> tuple[Action, str]:
    """
    Classifies a GCP API error into an actionable Slurm response.
    Returns: (Action Enum, Normalized Reason String)
    """
    msg_lower = error_message.lower()
    
    # 1. Quota Errors -> Transient (active usage limits), require REQUEUE
    if "quotaexceeded" in error_code.lower() or "quotaexceeded" in msg_lower or ("quota" in msg_lower and "exceeded" in msg_lower) or error_code == "QUOTA_EXCEEDED":
        return Action.REQUEUE, f"GCP Quota Exceeded: {error_message}"
        
    # 2. Capacity Errors -> Transient, require REQUEUE
    capacity_codes = ["ZONE_RESOURCE_POOL_EXHAUSTED", "VM_MIN_COUNT_NOT_REACHED", "INSUFFICIENT_RESOURCE_CAPACITY"]
    if any(code in error_code for code in capacity_codes) or "sufficient capacity" in msg_lower:
        return Action.REQUEUE, f"GCP Capacity Exhausted: {error_message}"
        
    # 3. Default fallback
    return Action.IGNORE, f"GCP Error [{error_code}]: {error_message}"
