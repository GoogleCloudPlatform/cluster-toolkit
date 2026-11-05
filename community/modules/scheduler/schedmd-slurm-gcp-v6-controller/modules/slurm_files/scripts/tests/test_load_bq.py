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
from mock import Mock, PropertyMock, patch
from google.api_core import exceptions
from google.cloud import bigquery as bq

from common import TstCfg  # needed to import util
import util
import load_bq


@pytest.fixture(autouse=True)
def reset_load_bq_globals():
    load_bq.client = None
    load_bq.dataset = None
    load_bq.table = None
    yield
    load_bq.client = None
    load_bq.dataset = None
    load_bq.table = None


@pytest.mark.parametrize(
    "raw_zone,expected_region",
    [
        ("projects/123456789/zones/europe-west9-a", "europe-west9"),
        ("https://www.googleapis.com/compute/v1/projects/my-proj/zones/asia-northeast1-b", "asia-northeast1"),
        ("us-central1-c", "us-central1"),
        ("", None),
        ("global", None),
    ],
)
def test_get_controller_region(raw_zone, expected_region):
    lkp = Mock()
    lkp.zone = raw_zone
    with patch.object(load_bq, "lookup", return_value=lkp):
        assert load_bq.get_controller_region() == expected_region


@pytest.mark.parametrize(
    "exc",
    [
        util.MetadataNotFoundError("no metadata server"),
        RuntimeError("unexpected failure"),
    ],
)
def test_get_controller_region_exception(exc):
    lkp = Mock()
    type(lkp).zone = PropertyMock(side_effect=exc)
    with patch.object(load_bq, "lookup", return_value=lkp):
        assert load_bq.get_controller_region() is None


def test_init_table_existing_dataset():
    cfg = TstCfg(slurm_cluster_name="m22", project="my-proj")
    lkp = util.Lookup(cfg)
    mock_client = Mock()
    dataset_ref = bq.DatasetReference(project="my-proj", dataset_id="m22_job_data")
    existing_ds = bq.Dataset(dataset_ref)
    mock_client.get_dataset.return_value = existing_ds
    mock_table = Mock()
    mock_client.create_table.return_value = mock_table
    mock_client.get_table.return_value = mock_table
    mock_client.update_table.return_value = mock_table

    with patch.object(load_bq, "lookup", return_value=lkp), \
         patch.object(load_bq.bq, "Client", return_value=mock_client), \
         patch.object(util, "default_credentials", return_value=Mock()), \
         patch.object(util, "create_client_options", return_value=Mock()):
        load_bq.init_table()

    mock_client.get_dataset.assert_called_once_with(dataset_ref)
    mock_client.create_dataset.assert_not_called()
    mock_client.create_table.assert_called_once()
    mock_client.get_table.assert_called_once()
    mock_client.update_table.assert_called_once_with(mock_table, ["schema"])
    assert load_bq.dataset is existing_ds


def test_init_table_missing_dataset_sets_controller_region():
    cfg = TstCfg(slurm_cluster_name="m22", project="my-proj")
    lkp = util.Lookup(cfg)
    mock_client = Mock()
    dataset_ref = bq.DatasetReference(project="my-proj", dataset_id="m22_job_data")
    created_ds = bq.Dataset(dataset_ref)
    created_ds.location = "europe-west9"
    mock_client.get_dataset.side_effect = exceptions.NotFound("Dataset not found")
    mock_client.create_dataset.return_value = created_ds
    mock_table = Mock()
    mock_client.create_table.return_value = mock_table
    mock_client.get_table.return_value = mock_table
    mock_client.update_table.return_value = mock_table

    with patch.object(load_bq, "lookup", return_value=lkp), \
         patch.object(load_bq, "get_controller_region", return_value="europe-west9"), \
         patch.object(load_bq.bq, "Client", return_value=mock_client), \
         patch.object(util, "default_credentials", return_value=Mock()), \
         patch.object(util, "create_client_options", return_value=Mock()):
        load_bq.init_table()

    mock_client.get_dataset.assert_called_once_with(dataset_ref)
    mock_client.create_dataset.assert_called_once()
    ds_arg = mock_client.create_dataset.call_args.args[0]
    assert isinstance(ds_arg, bq.Dataset)
    assert ds_arg.reference == dataset_ref
    assert ds_arg.location == "europe-west9"
    assert mock_client.create_dataset.call_args.kwargs == {"exists_ok": True}
    mock_client.create_table.assert_called_once()
    mock_client.get_table.assert_called_once()
    mock_client.update_table.assert_called_once_with(mock_table, ["schema"])


def test_init_table_missing_dataset_when_region_is_none():
    cfg = TstCfg(slurm_cluster_name="m22", project="my-proj")
    lkp = util.Lookup(cfg)
    mock_client = Mock()
    dataset_ref = bq.DatasetReference(project="my-proj", dataset_id="m22_job_data")
    mock_client.get_dataset.side_effect = exceptions.NotFound("Dataset not found")
    mock_client.create_dataset.return_value = bq.Dataset(dataset_ref)
    mock_table = Mock()
    mock_client.create_table.return_value = mock_table
    mock_client.get_table.return_value = mock_table
    mock_client.update_table.return_value = mock_table

    with patch.object(load_bq, "lookup", return_value=lkp), \
         patch.object(load_bq, "get_controller_region", return_value=None), \
         patch.object(load_bq.bq, "Client", return_value=mock_client), \
         patch.object(util, "default_credentials", return_value=Mock()), \
         patch.object(util, "create_client_options", return_value=Mock()):
        load_bq.init_table()

    mock_client.create_dataset.assert_called_once()
    ds_arg = mock_client.create_dataset.call_args.args[0]
    assert isinstance(ds_arg, bq.Dataset)
    assert ds_arg.reference == dataset_ref
    assert ds_arg.location is None
    assert mock_client.create_dataset.call_args.kwargs == {"exists_ok": True}


def test_init_table_idempotent():
    cfg = TstCfg(slurm_cluster_name="m22", project="my-proj")
    lkp = util.Lookup(cfg)
    mock_client = Mock()
    dataset_ref = bq.DatasetReference(project="my-proj", dataset_id="m22_job_data")
    existing_ds = bq.Dataset(dataset_ref)
    mock_client.get_dataset.return_value = existing_ds
    mock_table = Mock()
    mock_client.create_table.return_value = mock_table
    mock_client.get_table.return_value = mock_table
    mock_client.update_table.return_value = mock_table

    with patch.object(load_bq, "lookup", return_value=lkp), \
         patch.object(load_bq.bq, "Client", return_value=mock_client) as client_cls, \
         patch.object(util, "default_credentials", return_value=Mock()), \
         patch.object(util, "create_client_options", return_value=Mock()):
        load_bq.init_table()
        assert isinstance(load_bq.dataset, bq.Dataset)
        load_bq.init_table()

    client_cls.assert_called_once()
    assert mock_client.get_dataset.call_count == 2
    for call_args in mock_client.get_dataset.call_args_list:
        assert call_args.args[0] == dataset_ref


def test_main_disabled_exits_early():
    cfg = util.NSDict(enable_bigquery_load=False)
    lkp = util.Lookup(cfg)

    with patch.object(load_bq, "lookup", return_value=lkp), \
         patch.object(load_bq, "init_table") as mock_init_table, \
         patch.object(load_bq, "purge_job_idx_cache") as mock_purge, \
         patch.object(load_bq.bq, "Client") as mock_client_cls:
        with pytest.raises(SystemExit) as exc_info:
            load_bq.main()

    assert exc_info.value.code == 0
    mock_init_table.assert_not_called()
    mock_purge.assert_not_called()
    mock_client_cls.assert_not_called()
