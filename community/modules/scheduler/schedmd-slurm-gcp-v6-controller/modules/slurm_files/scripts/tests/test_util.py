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

from typing import Optional, Type

import pytest
from mock import Mock
from datetime import datetime, timezone, timedelta
import unittest

from common import TstNodeset, TstCfg # needed to import util
import util
from util import NodeState, MachineType, AcceleratorInfo, UpcomingMaintenance, InstanceResourceStatus, FutureReservation, ReservationDetails
from google.api_core.client_options import ClientOptions  # noqa: E402
from googleapiclient.errors import HttpError # type: ignore
from util import NSDict

# Note: need to install pytest-mock

@pytest.mark.parametrize(
    "name,expected",
    [
        (
            "az-buka-23",
            {
                "cluster": "az",
                "nodeset": "buka",
                "node": "23",
                "prefix": "az-buka",
                "range": None,
                "suffix": "23",
            },
        ),
        (
            "az-buka-xyzf",
            {
                "cluster": "az",
                "nodeset": "buka",
                "node": "xyzf",
                "prefix": "az-buka",
                "range": None,
                "suffix": "xyzf",
            },
        ),
        (
            "az-buka-[2-3]",
            {
                "cluster": "az",
                "nodeset": "buka",
                "node": "[2-3]",
                "prefix": "az-buka",
                "range": "[2-3]",
                "suffix": None,
            },
        ),
    ],
)
def test_node_desc(name, expected):
    cfg = TstCfg(
        slurm_cluster_name="az",
        nodeset={"buka": TstNodeset(nodeset_name="buka")},
    )
    lkp = util.Lookup(cfg)
    assert lkp._node_desc(name) == expected


@pytest.mark.parametrize(
    "name,expected",
    [
        ("az-buka-23", 23),
        ("az-buka-0", 0),
        ("az-buka", Exception),
        ("az-buka-xyzf", ValueError),
        ("az-buka-[2-3]", ValueError),
    ],
)
def test_node_index(name, expected):
    cfg = TstCfg(
        slurm_cluster_name="az",
        nodeset={"buka": TstNodeset(nodeset_name="buka")},
    )
    lkp = util.Lookup(cfg)
    if  type(expected) is type and issubclass(expected, Exception):
        with pytest.raises(expected):
            lkp.node_index(name) 
    else:
        assert lkp.node_index(name) == expected


@pytest.mark.parametrize(
    "name",
    [
        "az-buka",
    ],
)
def test_node_desc_fail(name):
    with pytest.raises(Exception):
        util.lookup()._node_desc(name)


@pytest.mark.parametrize(
    "names,expected",
    [
        ("pedro,pedro-1,pedro-2,pedro-01,pedro-02", "pedro,pedro-[1-2,01-02]"),
        ("pedro,,pedro-1,,pedro-2", "pedro,pedro-[1-2]"),
        ("pedro-8,pedro-9,pedro-10,pedro-11", "pedro-[8-9,10-11]"),
        ("pedro-08,pedro-09,pedro-10,pedro-11", "pedro-[08-11]"),
        ("pedro-08,pedro-09,pedro-8,pedro-9", "pedro-[8-9,08-09]"),
        ("pedro-10,pedro-08,pedro-09,pedro-8,pedro-9", "pedro-[8-9,08-10]"),
        ("pedro-8,pedro-9,juan-10,juan-11", "juan-[10-11],pedro-[8-9]"),
        ("az,buki,vedi", "az,buki,vedi"),
        ("a0,a1,a2,a3,a4,a5,a6,a7,a8,a9,a10,a11,a12", "a[0-9,10-12]"),
        ("a0,a2,a4,a6,a7,a8,a11,a12", "a[0,2,4,6-8,11-12]"),
        ("seas7-0,seas7-1", "seas7-[0-1]"),
    ],
)
def test_to_hostlist(names, expected):
    assert util.to_hostlist(names.split(",")) == expected


@pytest.mark.parametrize(
    "api,ep_ver,expected",
    [
        (
            util.ApiEndpoint.BQ,
            "v1",
            ClientOptions(api_endpoint="https://bq.googleapis.com/v1/"),
        ),
        (
            util.ApiEndpoint.COMPUTE,
            "staging_v1",
            ClientOptions(api_endpoint="https://compute.googleapis.com/staging_v1/"),
        ),
        (
            util.ApiEndpoint.SECRET,
            "v1",
            ClientOptions(api_endpoint="https://secret_manager.googleapis.com/v1/"),
        ),
        (
            util.ApiEndpoint.STORAGE,
            "beta",
            ClientOptions(api_endpoint="https://storage.googleapis.com/beta/"),
        ),
        (
            util.ApiEndpoint.TPU,
            "alpha",
            ClientOptions(api_endpoint="https://tpu.googleapis.com/alpha/"),
        ),
    ],
)
def test_create_client_options(
    api: util.ApiEndpoint, ep_ver: str, expected: ClientOptions, mocker
):
    ud_mock = mocker.patch("util.universe_domain")
    ep_mock = mocker.patch("util.endpoint_version")
    ud_mock.return_value = "googleapis.com"
    ep_mock.return_value = ep_ver
    assert util.create_client_options(api).__repr__() == expected.__repr__()



@pytest.mark.parametrize(
        "nodeset,err",
        [
            (TstNodeset(reservation_name="projects/x/reservations/y"), AssertionError), # no zones
            (TstNodeset(
                reservation_name="projects/x/reservations/y",
                zone_policy_allow=["eine", "zwei"]), AssertionError), # multiples zones
            (TstNodeset(
                reservation_name="robin",
                zone_policy_allow=["eine"]), ValueError), # invalid name
            (TstNodeset(
                reservation_name="projects/reservations/y",
                zone_policy_allow=["eine"]), ValueError), # invalid name
            (TstNodeset(
                reservation_name="projects/x/zones/z/reservations/y",
                zone_policy_allow=["eine"]), ValueError), # invalid name
        ]
)
def test_nodeset_reservation_err(nodeset, err):
    lkp = util.Lookup(TstCfg())
    lkp._get_reservation = Mock()
    with pytest.raises(err):
        lkp.nodeset_reservation(nodeset)
    lkp._get_reservation.assert_not_called() # type: ignore

@pytest.mark.parametrize(
        "nodeset,policies,expected",
        [
            (TstNodeset(), [], None), # no reservation
            (TstNodeset(
                reservation_name="projects/bobin/reservations/robin",
                zone_policy_allow=["eine"]),
                [],
                util.ReservationDetails(
                    project="bobin",
                    zone="eine",
                    name="robin",
                    policies=[],
                    deployment_type=None,
                    reservation_mode=None,
                    assured_count=0,
                    delete_at_time=None,
                    bulk_insert_name="projects/bobin/reservations/robin")),
            (TstNodeset(
                reservation_name="projects/bobin/reservations/robin",
                zone_policy_allow=["eine"]),
                ["seven/wanders", "five/red/apples", "yum"],
                util.ReservationDetails(
                    project="bobin",
                    zone="eine",
                    name="robin",
                    policies=["wanders", "apples", "yum"],
                    deployment_type=None,
                    reservation_mode=None,
                    assured_count=0,
                    delete_at_time=None,
                    bulk_insert_name="projects/bobin/reservations/robin")),
            (TstNodeset(
                reservation_name="projects/bobin/reservations/robin/snek/cheese-brie-6",
                zone_policy_allow=["eine"]),
                [],
                util.ReservationDetails(
                    project="bobin",
                    zone="eine",
                    name="robin",
                    policies=[],
                    deployment_type=None,
                    reservation_mode=None,
                    assured_count=0,
                    delete_at_time=None,
                    bulk_insert_name="projects/bobin/reservations/robin/snek/cheese-brie-6")),

        ])

def test_nodeset_reservation_ok(nodeset, policies, expected):
    lkp = util.Lookup(TstCfg())
    lkp._get_reservation = Mock()

    if not expected:
        assert lkp.nodeset_reservation(nodeset) is None
        lkp._get_reservation.assert_not_called() # type: ignore
        return

    lkp._get_reservation.return_value = { # type: ignore
        "resourcePolicies": {i: p for i, p in enumerate(policies)},
    }
    assert lkp.nodeset_reservation(nodeset) == expected
    lkp._get_reservation.assert_called_once_with(expected.project, expected.zone, expected.name) # type: ignore

@pytest.mark.parametrize(
    "job_info,expected_job",
    [
        (
            """JobId=123
            TimeLimit=02:00:00
            JobName=myjob
            JobState=PENDING
            ReqNodeList=node-[1-10]""",
            util.Job(
                id=123,
                duration=timedelta(days=0, hours=2, minutes=0, seconds=0),
                name="myjob",
                job_state="PENDING",
                required_nodes="node-[1-10]"
            ),
        ),
        (
            """JobId=456
            JobName=anotherjob
            JobState=PENDING
            ReqNodeList=node-group1""",
            util.Job(
                id=456,
                duration=None,
                name="anotherjob",
                job_state="PENDING",
                required_nodes="node-group1"
            ),
        ),
        (
            """JobId=789
            TimeLimit=00:30:00
            JobState=COMPLETED""",
            util.Job(
                id=789,
                duration=timedelta(minutes=30),
                name=None,
                job_state="COMPLETED",
                required_nodes=None
            ),
        ),
        (
            """JobId=101112
            TimeLimit=1-00:30:00
            JobState=COMPLETED,
            ReqNodeList=node-[1-10],grob-pop-[2,1,44-77]""",
            util.Job(
                id=101112,
                duration=timedelta(days=1, hours=0, minutes=30, seconds=0),
                name=None,
                job_state="COMPLETED",
                required_nodes="node-[1-10],grob-pop-[2,1,44-77]"
            ),
        ),
        (
            """JobId=131415
            TimeLimit=1-00:30:00
            JobName=mynode-1_maintenance
            JobState=COMPLETED,
            ReqNodeList=node-[1-10],grob-pop-[2,1,44-77]""",
            util.Job(
                id=131415,
                duration=timedelta(days=1, hours=0, minutes=30, seconds=0),
                name="mynode-1_maintenance",
                job_state="COMPLETED",
                required_nodes="node-[1-10],grob-pop-[2,1,44-77]"
            ),
        ),
    ],
)
def test_parse_job_info(job_info, expected_job):
    lkp = util.Lookup(TstCfg())
    assert lkp._parse_job_info(job_info) == expected_job



@pytest.mark.parametrize(
    "node,state,want",
    [
        ("c-n-2", NodeState("DOWN", frozenset([])), NodeState("DOWN", frozenset([]))), # happy scenario
        ("c-d-vodoo", None, None), # dynamic nodeset
        ("c-x-44", None, None), # unknown(removed) nodeset
        ("c-n-7", None, None), # Out of bounds: c-n-[0-4] - downsized nodeset
        ("c-t-7", None, None), # Out of bounds: c-t-[0-4] - downsized nodeset TPU
        ("c-n-2", None, RuntimeError), # something is wrong
        ("c-t-2", None, RuntimeError), # something is wrong, but TPU
        
        # Check boundaries match [0-5)
        ("c-n-5", None, None), # out of boundaries
        ("c-n-4", None, RuntimeError), # within boundaries
    ])
def test_node_state(node: str, state: Optional[NodeState], want: NodeState | None | Type[Exception]):
    cfg = TstCfg(
        slurm_cluster_name="c",
        nodeset={
            "n": TstNodeset(nodeset_name="n", node_count_static=2, node_count_dynamic_max=3)},
        nodeset_tpu={
            "t": TstNodeset(nodeset_name="t", node_count_static=2, node_count_dynamic_max=3)},
        nodeset_dyn={
            "d": TstNodeset(nodeset_name="d")},
    )
    lkp = util.Lookup(cfg)
    lkp.slurm_nodes = lambda: {node: state} if state else {} # type: ignore[assignment]
    # ... see https://github.com/python/typeshed/issues/6347    
        
    if  type(want) is type and issubclass(want, Exception):
        with pytest.raises(want):
            lkp.node_state(node)
    else:
        assert lkp.node_state(node) == want
        


@pytest.mark.parametrize(
    "jo,want",
    [
        ({
            "accelerators": [ { "guestAcceleratorCount": 1, "guestAcceleratorType": "nvidia-tesla-a100" } ],
            "creationTimestamp": "1969-12-31T16:00:00.000-08:00",
            "description": "Accelerator Optimized: 1 NVIDIA Tesla A100 GPU, 12 vCPUs, 85GB RAM",
            "guestCpus": 12,
            "id": "1000012",
            "imageSpaceGb": 0,
            "isSharedCpu": False,
            "kind": "compute#machineType",
            "maximumPersistentDisks": 128,
            "maximumPersistentDisksSizeGb": "263168",
            "memoryMb": 87040,
            "name": "a2-highgpu-1g",
            "selfLink": "https://www.googleapis.com/compute/v1/projects/io-playground/zones/us-central1-a/machineTypes/a2-highgpu-1g",
            "zone": "us-central1-a"
        }, MachineType(
            name="a2-highgpu-1g",
            guest_cpus=12,
            memory_mb=87040,
            accelerators=[
                AcceleratorInfo(type="nvidia-tesla-a100", count=1)
            ]
        )),
        ({
            "architecture": "X86_64",
            "creationTimestamp": "1969-12-31T16:00:00.000-08:00",
            "description": "8 vCPUs, 32 GB RAM",
            "guestCpus": 8,
            "id": "1210008",
            "imageSpaceGb": 0,
            "isSharedCpu": False,
            "kind": "compute#machineType",
            "maximumPersistentDisks": 128,
            "maximumPersistentDisksSizeGb": "263168",
            "memoryMb": 32768,
            "name": "t2d-standard-8",
            "selfLink": "https://www.googleapis.com/compute/v1/projects/io-playground/zones/europe-north2-b/machineTypes/t2d-standard-8",
            "zone": "europe-north2-b"
        }, MachineType(
            name="t2d-standard-8",
            guest_cpus=8,
            memory_mb=32768,
            accelerators=[]
        )),
    ])
def test_MachineType_from_json(jo: dict, want: MachineType):
    assert MachineType.from_json(jo) == want


@pytest.mark.parametrize(
    "template,expected",
    [
        (
            NSDict({
                "machine_type": MachineType(
                                    name="e2",
                                    guest_cpus=12,
                                    memory_mb=87040,
                                    accelerators=[]),
            }),
            None
        ),
        (
            NSDict({
                "machine_type": MachineType(
                                    name="tpu-machine",
                                    guest_cpus=12,
                                    memory_mb=87040,
                                    accelerators=[
                                        AcceleratorInfo(type="tpu-v6", count=1)
                                    ]),
            }),
            None
        ),
        (
            NSDict({
                "machine_type": MachineType(
                                    name="a2-highgpu-1g",
                                    guest_cpus=12,
                                    memory_mb=87040,
                                    accelerators=[AcceleratorInfo(type="nvidia-tesla-a100", count=1)]
                                    ),
            }),
            AcceleratorInfo(type="nvidia-tesla-a100", count=1)
        ),
        (
            NSDict({
                "machine_type": MachineType(
                                    name="a2-highgpu-1g",
                                    guest_cpus=12,
                                    memory_mb=87040,
                                    accelerators=[]),
                "guestAccelerators":[ { "acceleratorCount": 1, "acceleratorType": "nvidia-tesla-a100" } ],
            }),
            AcceleratorInfo(type="nvidia-tesla-a100", count=1)
        ),
    ],
)
def test_get_template_gpu(template, expected):
    assert util.get_template_gpu(template) == expected


UTC, PST = timezone.utc, timezone(timedelta(hours=-8))

@pytest.mark.parametrize(
    "got,want",
    [
        # from instance.creationTimestamp: 
        ("2024-11-30T12:47:51.676-08:00", datetime(2024, 11, 30, 12, 47, 51, 676000, tzinfo=PST)),
        # from futureReservation.creationTimestamp
        ("2024-11-05T15:23:33.702-08:00", datetime(2024, 11, 5, 15, 23, 33, 702000, tzinfo=PST)), 
        # from futureReservation.timeWindow.endTime
        ("2025-01-15T00:00:00Z", datetime(2025, 1, 15, 0, 0, tzinfo=UTC)),
        # fallback to UTC if no tz is specified
        ("2025-01-15T00:00:00", datetime(2025, 1, 15, 0, 0, tzinfo=UTC)),
    ])
def test_parse_gcp_timestamp(got: str, want: datetime):
    assert util.parse_gcp_timestamp(got) == want


@pytest.mark.parametrize(
    "got,want",
    [
        (None, None),
        (dict(
            windowStartTime="2025-01-15T00:00:00Z",
            somethingToIgnore="past failures",
        ), UpcomingMaintenance(window_start_time=datetime(2025, 1, 15, 0, 0, tzinfo=UTC))),
        (dict(
            startTimeWindow=dict(
                earliest="2025-01-15T00:00:00Z"),
            somethingToIgnore="past failures",
        ), UpcomingMaintenance(window_start_time=datetime(2025, 1, 15, 0, 0, tzinfo=UTC))),
        (dict(
            windowStartTime="2025-01-15T00:00:00Z",
            startTimeWindow=dict(
                earliest="2025-01-25T00:00:00Z"), # ignored
            somethingToIgnore="past failures",
        ), UpcomingMaintenance(window_start_time=datetime(2025, 1, 15, 0, 0, tzinfo=UTC))),
    ])
def tests_parse_UpcomingMaintenance_OK(got: dict, want: Optional[UpcomingMaintenance]):
    assert UpcomingMaintenance.from_json(got) == want


@pytest.mark.parametrize(
    "got",
    [
        {},
        dict(
            windowStartTime=dict(
                earliest="2025-01-15T00:00:00Z")),
    ])
def tests_parse_UpcomingMaintenance_FAIL(got: dict):
    with pytest.raises(ValueError):
            UpcomingMaintenance.from_json(got)


@pytest.mark.parametrize(
    "got,want",
    [
        (None,  InstanceResourceStatus(
            physical_host=None,
            upcoming_maintenance=None)),
        ({}, InstanceResourceStatus(
            physical_host=None,
            upcoming_maintenance=None)),
        (dict(
            physicalHost="/aaa/bbb/ccc"), 
        InstanceResourceStatus(
            physical_host="/aaa/bbb/ccc",
            upcoming_maintenance=None)),
        (dict(  # invalid upcomingMaintenance field to be ignored
            physicalHost="/aaa/bbb/ccc",
            upcomingMaintenance="maintenance is upon us"),
        InstanceResourceStatus(
            physical_host="/aaa/bbb/ccc",
            upcoming_maintenance=None)),
        (dict(
            physicalHost="/aaa/bbb/ccc",
            upcomingMaintenance=dict(windowStartTime="2025-01-15T00:00:00Z")), 
        InstanceResourceStatus(
            physical_host="/aaa/bbb/ccc",
            upcoming_maintenance=UpcomingMaintenance(
                window_start_time=datetime(2025, 1, 15, 0, 0, tzinfo=UTC)))),
    ])
def test_parse_InstanceResourceStatus(got: dict, want: Optional[InstanceResourceStatus]):
    assert InstanceResourceStatus.from_json(got) == want


@pytest.mark.parametrize(
    "link,component_name,expected",
    [
        (
            "mylink/regions/us-cental1/other",
            "regions",
            "us-cental1"
        ),
        (
            "mylink/global/other",
            "regions",
            None
        ),
    ],
)
def test_get_self_link_component(link, component_name, expected):
    assert util.get_self_link_component(link, component_name) == expected


def test_future_reservation_none():
    lkp = util.Lookup(TstCfg())
    assert lkp.future_reservation(TstNodeset()) == None


def test_future_reservation_declined():
    lkp = util.Lookup(TstCfg())
    lkp._get_future_reservation = Mock(return_value=dict(
        timeWindow = { "startTime": "2025-01-27T23:30:00Z", "endTime": "2025-02-03T23:30:00Z" },
        status = {"procurementStatus": "DECLINED"},
        reservationMode = "CALENDAR",
        specificReservationRequired = True,
    ))

    assert lkp.future_reservation(
        TstNodeset(future_reservation="projects/manhattan/zones/danger/futureReservations/zebra")) == FutureReservation(
            project='manhattan', 
            zone='danger', 
            name='zebra', 
            specific=True, 
            start_time=datetime(2025, 1, 27, 23, 30, tzinfo=timezone.utc), 
            end_time=datetime(2025, 2, 3, 23, 30, tzinfo=timezone.utc),
            reservation_mode="CALENDAR",
            active_reservation=None)
    lkp._get_future_reservation.assert_called_once_with("manhattan", "danger", "zebra")

@unittest.mock.patch('util.now', return_value=datetime(2025, 2, 13, 0, 0, tzinfo=timezone.utc))
def test_future_reservation_active(_):
    lkp = util.Lookup(TstCfg())
    lkp._get_future_reservation = Mock(return_value=dict(
        timeWindow = { "startTime": "2025-01-27T23:30:00Z", "endTime": "2025-02-21T23:30:00Z" },
        status = {
            "procurementStatus": "FULFILLED",
            "autoCreatedReservations": [
                "https://www.googleapis.com/compute/alpha/projects/manhattan/zones/danger/reservations/melon"
            ],
        },
        specificReservationRequired = True,
    ))
    lkp._get_reservation = Mock(return_value=dict())

    assert lkp.future_reservation(
        TstNodeset(future_reservation="projects/manhattan/zones/danger/futureReservations/zebra")) == FutureReservation(
            project='manhattan', 
            zone='danger', 
            name='zebra', 
            specific=True, 
            start_time=datetime(2025, 1, 27, 23, 30, tzinfo=timezone.utc), 
            end_time=datetime(2025, 2, 21, 23, 30, tzinfo=timezone.utc),
            reservation_mode=None, 
            active_reservation=ReservationDetails(
                project='manhattan',
                zone='danger',
                name='melon',
                policies=[],
                reservation_mode=None,
                assured_count=0,
                delete_at_time=None,
                bulk_insert_name="projects/manhattan/reservations/melon",
                deployment_type=None))
    
    lkp._get_future_reservation.assert_called_once_with("manhattan", "danger", "zebra")
    lkp._get_reservation.assert_called_once_with("manhattan", "danger", "melon")

@unittest.mock.patch('util.now', return_value=datetime(2025, 2, 28, 0, 0, tzinfo=timezone.utc))
def test_future_reservation_inactive(_):
    lkp = util.Lookup(TstCfg())
    lkp._get_future_reservation = Mock(return_value=dict(
        timeWindow = { "startTime": "2025-01-27T23:30:00Z", "endTime": "2025-02-21T23:30:00Z" },
        status = {
            "procurementStatus": "FULFILLED",
            "autoCreatedReservations": [
                "https://www.googleapis.com/compute/alpha/projects/manhattan/zones/danger/reservations/melon"
            ],
        },
        reservationMode = "DEFAULT",
        specificReservationRequired = True,
    ))
    lkp._get_reservation = Mock()

    assert lkp.future_reservation(
        TstNodeset(future_reservation="projects/manhattan/zones/danger/futureReservations/zebra")) == FutureReservation(
            project='manhattan', 
            zone='danger', 
            name='zebra', 
            specific=True, 
            start_time=datetime(2025, 1, 27, 23, 30, tzinfo=timezone.utc), 
            end_time=datetime(2025, 2, 21, 23, 30, tzinfo=timezone.utc), 
            reservation_mode="DEFAULT",
            active_reservation=None)
    
    lkp._get_future_reservation.assert_called_once_with("manhattan", "danger", "zebra")
    lkp._get_reservation.assert_not_called()

@pytest.mark.parametrize(
    "v1, v2, expected",
    [
        ("22.05", "21.08", True),
        ("21.08", "22.05", False),
        ("22.05", "22.05", True),
        ("22.05", "22.04", True),
        ("22.04", "22.05", False),
        ("22.05.1", "22.05", True),
        ("22.05", "22.05.1", True),
        ("22.05.1", "22.05.2", True),
        ("invalid", "22.05", False),
        ("22.05", "invalid", False),
        ("21.08.1", "22.05.3", False),
    ],
)
def test_slurm_version_gte(v1, v2, expected):
    assert util.slurm_version_gte(v1, v2) == expected

@pytest.mark.parametrize(
    "stdout_data, exception_to_raise, expected_version",
    [
        ("slurm 23.02.6", None, "23.02"),
        ("slurm version 24.11.0-pre1", None, "24.11"),
        ("Some other output", None, "unknown"),
        ("", None, "unknown"),
        (None, FileNotFoundError("slurmctld not found"), "unknown"),
        (None, Exception("simulated command failure"), "unknown"),
    ],
)
def test_slurm_version(stdout_data, exception_to_raise, expected_version, mocker):
    mock_run = mocker.patch("util.run")

    if exception_to_raise:
        mock_run.side_effect = exception_to_raise
    else:
        mock_run.return_value = mocker.Mock(stdout=stdout_data)

    lkp = util.Lookup(TstCfg())
    version = lkp.slurm_version
    
    assert version == expected_version
    mock_run.assert_called_once()


def test_get_reservation_details_403(mocker):
    lkp = util.Lookup(TstCfg())
    
    # Mock HttpError 403
    mock_resp = Mock()
    mock_resp.status = 403
    error = HttpError(mock_resp, b'Forbidden')
    
    lkp._get_reservation = Mock(side_effect=error)
    
    details = lkp.get_reservation_details(
        project="my-project",
        zone="us-central1-a",
        name="my-reservation",
        bulk_insert_name="projects/my-project/reservations/my-reservation"
    )
    
    assert details.policies == []
    assert details.deployment_type is None
    assert details.reservation_mode is None
    assert details.assured_count == 0
    assert details.bulk_insert_name == "projects/my-project/reservations/my-reservation"
    
    lkp._get_reservation.assert_called_once_with("my-project", "us-central1-a", "my-reservation")


def test_get_reservation_details_error(mocker):
    lkp = util.Lookup(TstCfg())
    
    # Mock HttpError 500
    mock_resp = Mock()
    mock_resp.status = 500
    error = HttpError(mock_resp, b'Internal Server Error')
    
    lkp._get_reservation = Mock(side_effect=error)
    
    with pytest.raises(HttpError):
        lkp.get_reservation_details(
            project="my-project",
            zone="us-central1-a",
            name="my-reservation",
            bulk_insert_name="projects/my-project/reservations/my-reservation"
        )
    
    lkp._get_reservation.assert_called_once_with("my-project", "us-central1-a", "my-reservation")


def test_batch_execute_custom_universe_domain(mocker):
    mocker.patch("util.universe_domain", return_value="apis-sovereign.goog")
    req0 = Mock()
    req0.execute.return_value = {"status": "DONE", "name": "op-0"}
    req1 = Mock()
    req1.execute.return_value = {"status": "DONE", "name": "op-1"}

    requests = {
        "node-0": req0,
        "node-1": req1,
    }
    done, failed = util.batch_execute(requests)
    assert len(done) == 2
    assert len(failed) == 0
    assert done["node-0"]["name"] == "op-0"
    assert done["node-1"]["name"] == "op-1"
    req0.execute.assert_called_once()
    req1.execute.assert_called_once()


def test_batch_execute_custom_universe_partial_failure(mocker):
    mocker.patch("util.universe_domain", return_value="apis-sovereign.goog")
    req_ok = Mock()
    req_ok.execute.return_value = {"status": "DONE"}
    req_err = Mock()
    req_err.execute.side_effect = RuntimeError("API error")

    requests = {
        "node-ok": req_ok,
        "node-err": req_err,
    }
    done, failed = util.batch_execute(requests)
    assert len(done) == 1
    assert len(failed) == 1
    assert "node-ok" in done
    assert "node-err" in failed
    req_ok.execute.assert_called_once()
    req_err.execute.assert_called_once()


def test_compute_service_default_universe_domain(mocker):
    mocker.patch("util.universe_domain", return_value="googleapis.com")
    mocker.patch("util.get_credentials", return_value=None)
    mocker.patch("util.get_dev_key", return_value=None)
    mock_build = mocker.patch("googleapiclient.discovery.build")

    util.compute_service()
    mock_build.assert_called_once()
    args, kwargs = mock_build.call_args
    assert args == ("compute", "beta")
    assert kwargs.get("discoveryServiceUrl") == "https://www.googleapis.com/discovery/v1/apis/{api}/{apiVersion}/rest"
    assert kwargs.get("static_discovery") is False
    assert kwargs.get("client_options") is None


def test_compute_service_custom_universe_domain(mocker):
    mocker.patch("util.universe_domain", return_value="apis-sovereign.goog")
    mocker.patch("util.get_credentials", return_value=None)
    mocker.patch("util.get_dev_key", return_value=None)
    mock_build = mocker.patch("googleapiclient.discovery.build")

    util.compute_service()
    mock_build.assert_called_once()
    args, kwargs = mock_build.call_args
    assert args == ("compute", "beta")
    assert kwargs.get("discoveryServiceUrl") is None
    assert kwargs.get("static_discovery") is True
    assert kwargs.get("client_options").api_endpoint == "https://compute.apis-sovereign.goog/compute/beta/"
    assert kwargs.get("client_options").universe_domain == "apis-sovereign.goog"


def test_is_mig_engine():
    cfg_bulk = TstCfg(provisioning_engine="BULK_INSERT")
    lkp_bulk = util.Lookup(cfg_bulk)
    assert not lkp_bulk.is_mig_engine()

    cfg_mig = TstCfg(provisioning_engine="MIG")
    lkp_mig = util.Lookup(cfg_mig)
    assert lkp_mig.is_mig_engine()


def test_mig_name():
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        provisioning_engine="MIG",
        nodeset={
            "ns1": TstNodeset(nodeset_name="ns1", node_count_dynamic_max=100),
            "ns2": TstNodeset(nodeset_name="ns2", node_count_dynamic_max=1200),
        }
    )
    lkp = util.Lookup(cfg)
    assert lkp.mig_name("ns1") == "testcl-ns1-mig-0"
    assert lkp.mig_name("ns2", index=0) == "testcl-ns2-mig-0"


def test_is_node_mig():
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        provisioning_engine="AUTO",
        nodeset={
            "ns_mig": TstNodeset(nodeset_name="ns_mig", mig_name="testcl-ns_mig-mig-0", provisioning_engine="MIG"),
            "ns_bulk": TstNodeset(nodeset_name="ns_bulk", provisioning_engine="BULK_INSERT"),
        }
    )
    lkp = util.Lookup(cfg)
    assert lkp.is_nodeset_mig("ns_mig")
    assert not lkp.is_nodeset_mig("ns_bulk")
    assert lkp.is_node_mig("testcl-ns_mig-0")
    assert not lkp.is_node_mig("testcl-ns_bulk-0")

    # Cluster-level MIG with explicit NodeSet-level BULK_INSERT override
    cfg_cluster_mig = TstCfg(
        slurm_cluster_name="testcl",
        provisioning_engine="MIG",
        nodeset={
            "ns_mig_cluster": TstNodeset(nodeset_name="ns_mig_cluster"),
            "ns_bulk_override": TstNodeset(nodeset_name="ns_bulk_override", provisioning_engine="BULK_INSERT"),
        }
    )
    lkp_cluster = util.Lookup(cfg_cluster_mig)
    assert lkp_cluster.is_nodeset_mig("ns_mig_cluster")
    assert not lkp_cluster.is_nodeset_mig("ns_bulk_override")
    assert lkp_cluster.is_node_mig("testcl-ns_mig_cluster-0")
    assert not lkp_cluster.is_node_mig("testcl-ns_bulk_override-0")

    # Cross-NodeSet Isolation: Legacy NodeSet without provisioning_engine in a hybrid cluster
    cfg_hybrid = TstCfg(
        slurm_cluster_name="testcl",
        provisioning_engine="AUTO",
        nodeset={
            "ns_mig": TstNodeset(nodeset_name="ns_mig", mig_name="testcl-ns_mig-mig-0", provisioning_engine="MIG"),
            "ns_legacy": TstNodeset(nodeset_name="ns_legacy"),  # No provisioning_engine field
        }
    )
    lkp_hybrid = util.Lookup(cfg_hybrid)
    assert lkp_hybrid.is_nodeset_mig("ns_mig")
    assert not lkp_hybrid.is_nodeset_mig("ns_legacy")
    assert lkp_hybrid.is_node_mig("testcl-ns_mig-0")
    assert not lkp_hybrid.is_node_mig("testcl-ns_legacy-0")


def test_get_mig_repairing_instances_empty_filter():
    cfg = TstCfg(slurm_cluster_name="testcl")
    lkp = util.Lookup(cfg)
    lkp.get_mig_instances = unittest.mock.MagicMock(return_value={
        "managedInstances": [
            {"currentAction": "REPAIRING", "name": "node-1"},
            {"currentAction": "RECREATING", "instance": "https://.../instances/node-2"},
            {"currentAction": "RESTARTING", "name": "node-4"},
            {"currentAction": "REPAIRING", "name": None, "instance": None},  # Ephemeral unassigned
            {"currentAction": "NONE", "name": "node-3"},
        ]
    })
    res = lkp.get_mig_repairing_instances("p", "r", "mig-0")
    assert res == {"node-1", "node-2", "node-4"}
    assert "" not in res


def test_is_provisioning_flex_node(monkeypatch):
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        project="testproj",
        nodeset={
            "flex_ns": TstNodeset(
                nodeset_name="flex_ns",
                region="us-central1",
                zone_policy_allow=["us-central1-a"],
                instance_template="projects/testproj/global/instanceTemplates/flex-tpl",
                dws_flex=NSDict(enabled=True, use_bulk_insert=False),
            ),
            "non_flex_ns": TstNodeset(
                nodeset_name="non_flex_ns",
                region="us-central1",
                dws_flex=NSDict(enabled=False),
            ),
        },
    )
    lkp = util.Lookup(cfg)

    # Non-flex node returns False immediately
    assert not lkp.is_provisioning_flex_node("testcl-non_flex_ns-0")

    # Mock instances call returning None (node not created yet)
    monkeypatch.setattr(lkp, "instance", lambda node: None)

    # Mock get_mig_list returning MIGs with versioned template
    fake_migs = {
        "items": [
            {
                "selfLink": "https://www.googleapis.com/compute/v1/projects/testproj/regions/us-central1/instanceGroupManagers/unrelated-mig",
                "versions": [{"instanceTemplate": "projects/testproj/global/instanceTemplates/other-tpl"}],
                "currentActions": {"creating": 1},
            },
            {
                "selfLink": "https://www.googleapis.com/compute/v1/projects/testproj/regions/us-central1/instanceGroupManagers/job-mig",
                "versions": [{"instanceTemplate": "projects/testproj/global/instanceTemplates/flex-tpl"}],
                "currentActions": {"creating": 1},
            },
        ]
    }
    monkeypatch.setattr(lkp, "get_mig_list", lambda proj, reg: fake_migs)

    def mock_get_mig_instances(proj, reg, mig_name):
        if mig_name == "job-mig":
            return {
                "managedInstances": [
                    {"instance": "projects/testproj/zones/us-central1-a/instances/testcl-flex_ns-0", "currentAction": "CREATING"}
                ]
            }
        return {"managedInstances": []}

    monkeypatch.setattr(lkp, "get_mig_instances", mock_get_mig_instances)

    assert lkp.is_provisioning_flex_node("testcl-flex_ns-0")
    assert lkp.is_provisioning_flex_node("testcl-flex_ns-0.c.testproj.internal")
    assert not lkp.is_provisioning_flex_node("testcl-flex_ns-1")


def test_is_nodeset_mig():
    import types
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        nodeset={
            "static_mig_ns": TstNodeset(nodeset_name="static_mig_ns", provisioning_engine="MIG", mig_name="testcl-static_mig_ns-mig-0"),
            "flex_ns": TstNodeset(nodeset_name="flex_ns", provisioning_engine="MIG", dws_flex=types.SimpleNamespace(enabled=True)),
            "bulk_ns": TstNodeset(nodeset_name="bulk_ns", provisioning_engine="BULK_INSERT"),
            "bulk_override_ns": TstNodeset(nodeset_name="bulk_override_ns", provisioning_engine="BULK_INSERT", mig_name="testcl-bulk_override_ns-mig-0"),
        },
    )
    lkp = util.Lookup(cfg)
    assert lkp.is_nodeset_mig("static_mig_ns") is True
    assert lkp.is_nodeset_mig("flex_ns") is False
    assert lkp.is_nodeset_mig("bulk_ns") is False
    assert lkp.is_nodeset_mig("bulk_override_ns") is False


def test_mig_name_multi_mig():
    cfg = TstCfg(
        slurm_cluster_name="testcl",
        nodeset={
            "small_ns": TstNodeset(nodeset_name="small_ns", node_count_static=50, node_count_dynamic_max=0),
            "large_static_ns": TstNodeset(nodeset_name="large_static_ns", node_count_static=2500, node_count_dynamic_max=0),
            "large_dynamic_ns": TstNodeset(nodeset_name="large_dynamic_ns", node_count_static=0, node_count_dynamic_max=1500),
        },
    )
    lkp = util.Lookup(cfg)

    # <= 1000 nodes -> indexed MIG name starting at 0
    assert lkp.mig_name("small_ns") == "testcl-small_ns-mig-0"
    assert lkp.node_mig_name("testcl-small_ns-0") == "testcl-small_ns-mig-0"
    assert lkp.node_mig_name("testcl-small_ns-49") == "testcl-small_ns-mig-0"

    # > 1000 static nodes -> indexed MIG names
    assert lkp.mig_name("large_static_ns", index=0) == "testcl-large_static_ns-mig-0"
    assert lkp.mig_name("large_static_ns", index=1) == "testcl-large_static_ns-mig-1"
    assert lkp.mig_name("large_static_ns", index=2) == "testcl-large_static_ns-mig-2"
    assert lkp.node_mig_name("testcl-large_static_ns-0") == "testcl-large_static_ns-mig-0"
    assert lkp.node_mig_name("testcl-large_static_ns-999") == "testcl-large_static_ns-mig-0"
    assert lkp.node_mig_name("testcl-large_static_ns-1000") == "testcl-large_static_ns-mig-1"
    assert lkp.node_mig_name("testcl-large_static_ns-1999") == "testcl-large_static_ns-mig-1"
    assert lkp.node_mig_name("testcl-large_static_ns-2000") == "testcl-large_static_ns-mig-2"

    # > 1000 dynamic nodes -> indexed MIG names
    assert lkp.node_mig_name("testcl-large_dynamic_ns-500") == "testcl-large_dynamic_ns-mig-0"
    assert lkp.node_mig_name("testcl-large_dynamic_ns-1200") == "testcl-large_dynamic_ns-mig-1"


@unittest.mock.patch("util.ensure_execute")
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
def test_suspend_mig_nodes_multi_mig(mock_compute_prop, mock_execute):
    import suspend

    cfg = TstCfg(
        slurm_cluster_name="testcl",
        project="testproj",
        nodeset={
            "large_ns": TstNodeset(nodeset_name="large_ns", region="us-central1", node_count_static=2500, node_count_dynamic_max=0),
        },
    )
    lkp = util.Lookup(cfg)
    mock_compute = unittest.mock.MagicMock()
    mock_compute_prop.return_value = mock_compute
    mock_execute.side_effect = lambda req: req.execute()
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
        "managedInstances": [
            {"instance": "projects/testproj/zones/us-central1-a/instances/testcl-large_ns-0"},
            {"instance": "projects/testproj/zones/us-central1-a/instances/testcl-large_ns-1005"},
        ]
    }

    # Pass duplicate entries ("testcl-large_ns-0" twice) and absent node "testcl-large_ns-999"; duplicates and absent nodes must be safely handled
    suspend.suspend_mig_nodes(["testcl-large_ns-0.c.testproj.internal", "testcl-large_ns-0", "testcl-large_ns-999", "testcl-large_ns-1005"], lkp=lkp)

    delete_calls = mock_compute.regionInstanceGroupManagers().deleteInstances.call_args_list
    assert len(delete_calls) == 2
    # Group 0 for node index 0 (FQDN normalized to short name, node 999 is skipped from body)
    assert delete_calls[0].kwargs["instanceGroupManager"] == "testcl-large_ns-mig-0"
    assert delete_calls[0].kwargs["body"]["instances"] == ["projects/testproj/zones/us-central1-a/instances/testcl-large_ns-0"]
    assert delete_calls[0].kwargs["body"]["skipInstancesOnValidationError"] is True
    # Group 1 for node index 1005
    assert delete_calls[1].kwargs["instanceGroupManager"] == "testcl-large_ns-mig-1"
    assert delete_calls[1].kwargs["body"]["instances"] == ["projects/testproj/zones/us-central1-a/instances/testcl-large_ns-1005"]
    assert delete_calls[1].kwargs["body"]["skipInstancesOnValidationError"] is True

    pic_del_calls = mock_compute.regionInstanceGroupManagers().deletePerInstanceConfigs.call_args_list
    assert len(pic_del_calls) == 2
    assert pic_del_calls[0].kwargs["instanceGroupManager"] == "testcl-large_ns-mig-0"
    assert set(pic_del_calls[0].kwargs["body"]["names"]) == {"testcl-large_ns-0", "testcl-large_ns-999"}
    assert pic_del_calls[1].kwargs["instanceGroupManager"] == "testcl-large_ns-mig-1"
    assert set(pic_del_calls[1].kwargs["body"]["names"]) == {"testcl-large_ns-1005"}


@unittest.mock.patch.object(util.Lookup, "instance", return_value=None)
@unittest.mock.patch.object(util.Lookup, "compute", new_callable=unittest.mock.PropertyMock)
@unittest.mock.patch("slurmsync.lookup")
def test_slurmsync_mig_auto_repair(mock_lookup, mock_compute_prop, mock_inst):
    import slurmsync

    cfg = TstCfg(
        slurm_cluster_name="testcl",
        project="testproj",
        provisioning_engine="MIG",
        nodeset={
            "ns": TstNodeset(nodeset_name="ns", region="us-central1", provisioning_engine="MIG", node_count_static=10, node_count_dynamic_max=0),
        },
    )
    lkp = util.Lookup(cfg)
    mock_compute = unittest.mock.MagicMock()
    mock_compute_prop.return_value = mock_compute
    mock_compute.regionInstanceGroupManagers().listManagedInstances().execute.return_value = {
        "managedInstances": [
            {"instance": "projects/testproj/zones/us-central1-a/instances/testcl-ns-0", "currentAction": "REPAIRING"},
            {"name": "testcl-ns-1", "currentAction": "REPAIRING"},
            {"instance": "projects/testproj/zones/us-central1-a/instances/testcl-ns-2", "currentAction": "NONE"},
        ]
    }
    mock_lookup.return_value = lkp

    # Verify set cache
    assert lkp.get_mig_repairing_instances("testproj", "us-central1", "testcl-ns-mig-0") == {"testcl-ns-0", "testcl-ns-1"}

    # When node exists in Slurm and is IDLE (not DOWN) -> NodeActionDown
    with unittest.mock.patch.object(util.Lookup, "node_state", return_value=slurmsync.NodeState(base="IDLE", flags=frozenset())):
        action0 = slurmsync.get_node_action("testcl-ns-0")
        assert isinstance(action0, slurmsync.NodeActionDown)
        assert "MIG Auto-Healing" in action0.reason

        # Test resolution when API returns 'name' field
        action1 = slurmsync.get_node_action("testcl-ns-1.c.testproj.internal")
        assert isinstance(action1, slurmsync.NodeActionDown)
        assert "MIG Auto-Healing" in action1.reason

    # When node does not exist in Slurm (node_state returns None) -> NodeActionUnchanged (don't run scontrol on invalid node)
    with unittest.mock.patch.object(util.Lookup, "node_state", return_value=None):
        action_none = slurmsync.get_node_action("testcl-ns-0")
        assert isinstance(action_none, slurmsync.NodeActionUnchanged)

    # When node was marked DOWN due to auto-healing and instance is now RUNNING -> NodeActionIdle
    with unittest.mock.patch.object(util.Lookup, "instance", return_value=unittest.mock.MagicMock(status="RUNNING")):
        with unittest.mock.patch.object(util.Lookup, "node_state", return_value=slurmsync.NodeState(base="DOWN", flags=frozenset())):
            with unittest.mock.patch("slurmsync.get_node_reason", return_value="MIG Auto-Healing instance repair in progress") as mock_get_reason:
                action_recovered = slurmsync.get_node_action("testcl-ns-2")
                assert isinstance(action_recovered, slurmsync.NodeActionIdle)
                mock_get_reason.assert_called_with("testcl-ns-2")

                # Verify FQDN is normalized to short name when calling get_node_reason
                action_recovered_fqdn = slurmsync.get_node_action("testcl-ns-2.c.testproj.internal")
                assert isinstance(action_recovered_fqdn, slurmsync.NodeActionIdle)
                mock_get_reason.assert_called_with("testcl-ns-2")
