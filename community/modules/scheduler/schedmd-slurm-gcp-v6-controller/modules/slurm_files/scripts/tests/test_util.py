import sys
import pytest

if ".." not in sys.path:
    sys.path.append("..")  # TODO: make this more robust
import util
from google.api_core.client_options import ClientOptions  # noqa: E402

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
    assert util.lkp._node_desc(name) == expected


@pytest.mark.parametrize(
    "name",
    [
        "az-buka",
    ],
)
def test_node_desc_fail(name):
    with pytest.raises(Exception):
        util.lkp._node_desc(name)


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
def test_to_hostlist_fast(names, expected):
    assert util.to_hostlist_fast(names.split(",")) == expected


@pytest.mark.parametrize(
    "api,ep_ver,expected",
    [
        (
            util.ApiEndpoint.BQ,
            "v1",
            ClientOptions(
                api_endpoint="https://bq.googleapis.com/v1/",
                universe_domain="googleapis.com",
            ),
        ),
        (
            util.ApiEndpoint.COMPUTE,
            "staging_v1",
            ClientOptions(
                api_endpoint="https://compute.googleapis.com/staging_v1/",
                universe_domain="googleapis.com",
            ),
        ),
        (
            util.ApiEndpoint.SECRET,
            "v1",
            ClientOptions(
                api_endpoint="https://secret_manager.googleapis.com/v1/",
                universe_domain="googleapis.com",
            ),
        ),
        (
            util.ApiEndpoint.STORAGE,
            "beta",
            ClientOptions(
                api_endpoint="https://storage.googleapis.com/beta/",
                universe_domain="googleapis.com",
            ),
        ),
        (
            util.ApiEndpoint.TPU,
            "alpha",
            ClientOptions(
                api_endpoint="https://tpu.googleapis.com/alpha/",
                universe_domain="googleapis.com",
            ),
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
    co = util.create_client_options(api)
    assert (
        co.api_endpoint == expected.api_endpoint
        and co.universe_domain == expected.universe_domain
    )
    ud_mock.return_value = None
    ep_mock.return_value = None
    co = util.create_client_options(api)
    assert (
        co.api_endpoint != expected.api_endpoint
        and co.universe_domain != expected.universe_domain
    )
