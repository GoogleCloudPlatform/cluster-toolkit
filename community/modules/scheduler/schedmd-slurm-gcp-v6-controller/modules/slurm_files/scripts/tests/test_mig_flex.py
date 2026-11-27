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

from unittest import mock

import pytest

from common import TstCfg, TstNodeset  # needed to import util, etc.
import util
import mig_flex


def _lkp(nodeset) -> mock.MagicMock:
    lkp = mock.MagicMock()
    lkp.cfg = TstCfg(slurm_cluster_name="m22")
    lkp.project = "p"
    lkp.node_nodeset.return_value = nodeset
    lkp.node_region.return_value = "us-central1"
    return lkp


def _insert_body(lkp) -> dict:
    return lkp.compute.regionInstanceGroupManagers().insert.call_args.kwargs["body"]


@pytest.mark.parametrize(
    "shape",
    [
        None,  # nodeset predating zone_target_shape
        "ANY_SINGLE_ZONE",
        "ANY",
        "BALANCED",
    ],
)
def test_resume_flex_chunk_target_shape_is_any_single_zone(shape):
    # Regional MIG resize requests (Flex-start) require ANY_SINGLE_ZONE, so the
    # nodeset's zone_target_shape must never leak into the DWS Flex MIG body.
    nodeset = TstNodeset(
        nodeset_name="flex",
        zone_policy_allow=["us-central1-a", "us-central1-b"],
        dws_flex=util.NSDict(
            {"enabled": True, "max_run_duration": 3600, "use_job_duration": False}
        ),
    )
    if shape is not None:
        nodeset.zone_target_shape = shape  # type: ignore[attr-defined]

    lkp = _lkp(nodeset)
    with mock.patch.object(util, "wait_for_operation", return_value={}), mock.patch.object(
        util, "log_api_request"
    ):
        mig_flex.resume_flex_chunk(["m22-flex-0"], None, lkp)

    assert _insert_body(lkp)["distributionPolicy"]["targetShape"] == "ANY_SINGLE_ZONE"
