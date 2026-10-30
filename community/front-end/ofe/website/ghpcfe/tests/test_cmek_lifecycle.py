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
"""A resource's key across its lifecycle: create, recreate, delete, failure.

- Cloud KMS never frees a CryptoKey name, so a Filestore or image recreated
  under the same name must not derive the key name its predecessor used.
- Deleting an image keeps its record unless the builder env and the key group
  were both torn down, so a failed delete does not orphan the key.
- A key that cannot be generated stops the deploy instead of being logged and
  skipped, which would deploy a stale cluster.yaml.
"""
import json
import tempfile
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

from django.test import RequestFactory, SimpleTestCase

from ..cluster_manager import cmek

ROOT = Path(__file__).resolve().parents[1]
CONFIG = {"cmek": {"enforcement": "preferred", "key_ring_name": "ofe-cmek"}}
CREDENTIAL = SimpleNamespace(detail=json.dumps({"project_id": "proj"}))


def _cmek_fields():
    return dict(
        cmek_enabled=True, cmek_key_name="", cmek_key_ring_name="",
        cmek_key_project_id="", cmek_deletion_policy="ABANDON",
        cmek_protection_level="", cmek_rotation_period="",
        cmek_encryption_service_account="", cloud_credential=CREDENTIAL,
        save=lambda **kwargs: None)


class DerivedKeyNamesAreNotReusedTests(SimpleTestCase):
    """Two resources with one name get two keys; one resource keeps its key."""

    def _filestore(self, name):
        return SimpleNamespace(
            name=name, cloud_zone="europe-west4-a",
            cloud_region="europe-west4", capacity=1024,
            vpc=SimpleNamespace(cloud_id="net"),
            exports=SimpleNamespace(first=lambda: SimpleNamespace(
                export_name="/share")),
            get_performance_tier_display=lambda: "BASIC_HDD",
            **_cmek_fields())

    def _write_filestore(self, fs):
        from ..cluster_manager import filesystem
        with mock.patch.object(filesystem.utils, "load_config",
                               return_value=CONFIG), \
                mock.patch.object(cmek, "ring_already_exists",
                                  return_value=False):
            filesystem.write_filestore_yaml(fs, Path(tempfile.mkdtemp()))
        return fs.cmek_key_name

    def _image(self, name):
        return SimpleNamespace(
            id=7, name=name, family="fam", cloud_zone="europe-west4-a",
            cloud_region="europe-west4", source_image_project="p",
            source_image_family="f", enable_os_login="TRUE",
            block_project_ssh_keys="TRUE",
            startup_script=SimpleNamespace(all=lambda: []),
            **_cmek_fields())

    def _write_image(self, img):
        from ..cluster_manager import image
        backend = image.ImageBackend.__new__(image.ImageBackend)
        backend.config = CONFIG
        backend.image = img
        backend.image_dir = Path(tempfile.mkdtemp())
        backend.blueprint_name = f"image_{img.id}"
        with mock.patch.object(cmek, "ring_already_exists",
                               return_value=False):
            backend._create_blueprint()
        self.assertIn(img.cmek_key_name,
                      (backend.image_dir / "image.yaml").read_text())
        return img.cmek_key_name

    def _assert_unique_and_stable(self, make, write, name):
        first, second = make(name), make(name)
        first_key = write(first)
        self.assertRegex(first_key, cmek.KMS_NAME_RE)
        self.assertNotEqual(first_key, write(second))
        # a retry or update reads the stored name back, minting nothing new
        self.assertEqual(write(first), first_key)

    def test_filestores_with_one_name_get_different_keys(self):
        self._assert_unique_and_stable(
            self._filestore, self._write_filestore, "home")

    def test_images_with_one_name_get_different_keys(self):
        self._assert_unique_and_stable(self._image, self._write_image, "img")

    def test_a_longest_filestore_name_still_yields_a_unique_valid_key(self):
        # models.Filesystem.name max_length=40
        self._assert_unique_and_stable(
            self._filestore, self._write_filestore, "f" * 40)

    def test_a_longest_image_name_still_yields_a_unique_valid_key(self):
        # models.Image.name max_length=30
        self._assert_unique_and_stable(
            self._image, self._write_image, "i" * 30)


class DeleteImageTearsDownBothGroupsTests(SimpleTestCase):
    """A builder-env failure must not stop the key group's teardown."""

    def _delete(self, builder_error=None, with_builder_dir=True):
        from ..cluster_manager import image
        backend = image.ImageBackend.__new__(image.ImageBackend)
        backend.image = SimpleNamespace(
            name="img", cloud_zone="europe-west4-a",
            cloud_credential=CREDENTIAL)
        backend.image_dir = Path(tempfile.mkdtemp())
        backend.blueprint_name = "image_7"
        backend.credentials_file = backend.image_dir / "cloud_credentials"
        if with_builder_dir:
            # initialised, so there may be something to destroy
            (backend.image_dir / "image_7/builder-env/.terraform").mkdir(
                parents=True)
        builder = mock.Mock(side_effect=builder_error)
        key = mock.Mock()
        with mock.patch.object(image.compute_v1, "ImagesClient"), \
                mock.patch.dict(image.os.environ), \
                mock.patch.object(backend, "_destroy_builder_env", builder), \
                mock.patch.object(backend, "_destroy_cmek_env", key):
            result = backend.delete_image()
        return result, builder, key

    def test_a_builder_failure_still_tears_down_the_key_and_fails(self):
        with self.assertLogs(level="ERROR"):
            result, builder, key = self._delete(RuntimeError("tf destroy"))
        builder.assert_called_once()
        key.assert_called_once()
        self.assertIs(result, False)

    def test_both_teardowns_succeeding_reports_success(self):
        result, builder, key = self._delete()
        builder.assert_called_once()
        key.assert_called_once()
        self.assertIs(result, True)

    def test_a_missing_builder_env_counts_as_torn_down(self):
        result, builder, key = self._delete(with_builder_dir=False)
        builder.assert_not_called()
        key.assert_called_once()
        self.assertIs(result, True)


def _image_backend():
    from ..cluster_manager import image
    backend = image.ImageBackend.__new__(image.ImageBackend)
    backend.image = SimpleNamespace(
        id=7, name="img", cloud_zone="europe-west4-a",
        cloud_credential=CREDENTIAL)
    backend.image_dir = Path(tempfile.mkdtemp())
    backend.blueprint_name = "image_7"
    backend.credentials_file = backend.image_dir / "cloud_credentials"
    backend.update_image_status = mock.Mock()
    return backend


def _group_dir(backend, group, state=None):
    """A group directory as `ghpc create` writes it, plus `state` if given."""
    group_dir = backend.image_dir / "image_7" / group
    group_dir.mkdir(parents=True)
    (group_dir / "main.tf").write_text("")
    if state == ".terraform":
        (group_dir / state).mkdir()
    elif state:
        (group_dir / state).write_text("{}")
    return str(group_dir)


class DeleteImageSkipsGroupsNothingWasAppliedFromTests(SimpleTestCase):
    """A build that failed before `terraform init` must stay deletable."""

    def _delete(self, backend, destroy_error=None):
        from ..cluster_manager import image
        with mock.patch.object(image.compute_v1, "ImagesClient"), \
                mock.patch.dict(image.os.environ), \
                mock.patch.object(image.utils, "run_terraform",
                                  side_effect=destroy_error) as tf:
            result = backend.delete_image()
        return result, [c.args[0] for c in tf.call_args_list]

    def test_an_uninitialised_builder_env_is_not_destroyed(self):
        backend = _image_backend()
        _group_dir(backend, "builder-env")
        result, destroyed = self._delete(backend)
        self.assertEqual(destroyed, [])
        self.assertIs(result, True)

    def test_an_uninitialised_key_group_is_not_destroyed(self):
        backend = _image_backend()
        builder = _group_dir(backend, "builder-env", "terraform.tfstate")
        _group_dir(backend, "image-cmek")
        result, destroyed = self._delete(backend)
        self.assertEqual(destroyed, [builder])
        self.assertIs(result, True)

    def test_an_applied_group_whose_destroy_fails_still_fails(self):
        from ..cluster_manager import image
        for group in ("builder-env", "image-cmek"):
            for state in (".terraform", "terraform.tfstate",
                          "terraform.tfstate.backup"):
                with self.subTest(group=group, state=state):
                    backend = _image_backend()
                    group_dir = _group_dir(backend, group, state)
                    with self.assertLogs(level="ERROR"):
                        result, destroyed = self._delete(
                            backend, image.subprocess.CalledProcessError(
                                1, "terraform destroy"))
                    self.assertEqual(destroyed, [group_dir])
                    self.assertIs(result, False)

    def test_the_view_keeps_the_row_when_an_applied_group_fails(self):
        from ..cluster_manager import image
        from ..views import images as v
        backend = _image_backend()
        _group_dir(backend, "image-cmek", "terraform.tfstate")
        img = mock.MagicMock(source_image_project="debian-cloud",
                             cmek_enabled=True, status_message="")
        view = v.ImageDeleteView()
        view.kwargs = {"pk": 1}
        with mock.patch.object(v, "get_object_or_404", return_value=img), \
                mock.patch.object(v, "ImageBackend", return_value=backend), \
                mock.patch.object(image.compute_v1, "ImagesClient"), \
                mock.patch.dict(image.os.environ), \
                mock.patch.object(
                    image.utils, "run_terraform",
                    side_effect=image.subprocess.CalledProcessError(
                        1, "terraform destroy")), \
                self.assertLogs(level="ERROR"):
            response = view.post(RequestFactory().post("/"))
        img.delete.assert_not_called()
        self.assertIs(json.loads(response.content)["success"], False)


class DeleteImageWithoutCredentialsTests(SimpleTestCase):
    """A build that failed before writing its credentials file stays
    deletable; a built image is not given up on for want of one."""

    def _delete(self, status, error):
        from google.auth.exceptions import DefaultCredentialsError
        from ..cluster_manager import image
        backend = _image_backend()
        backend.image.status = status
        _group_dir(backend, "image-cmek", "terraform.tfstate")
        cause = {"missing": DefaultCredentialsError(
                     f"File {backend.credentials_file} was not found."),
                 "unreadable": FileNotFoundError(
                     str(backend.credentials_file))}[error]
        with mock.patch.object(image.compute_v1, "ImagesClient",
                               side_effect=cause) as client, \
                mock.patch.dict(image.os.environ), \
                mock.patch.object(image.utils, "run_terraform") as tf, \
                self.assertLogs(level="WARNING") as logs:
            result = backend.delete_image()
        client.assert_called_once()
        return result, [c.args[1] for c in tf.call_args_list], logs.output

    def test_an_image_never_built_is_still_torn_down(self):
        for status in ("n", "e"):
            for error in ("missing", "unreadable"):
                with self.subTest(status=status, error=error):
                    result, commands, _logs = self._delete(status, error)
                    self.assertIs(result, True)
                    # the key group still goes
                    self.assertEqual(commands, ["destroy"])

    def test_a_built_image_without_credentials_fails_and_keeps_its_key(self):
        for error in ("missing", "unreadable"):
            with self.subTest(error=error):
                result, commands, logs = self._delete("r", error)
                self.assertIs(result, False)
                self.assertEqual(commands, [])
                self.assertTrue(any("ERROR" in line for line in logs))

    def test_the_view_answers_instead_of_failing(self):
        from google.auth.exceptions import DefaultCredentialsError
        from ..cluster_manager import image
        from ..views import images as v
        backend = _image_backend()
        backend.image.status = "e"
        img = mock.MagicMock(source_image_project="debian-cloud",
                             cmek_enabled=True, status="e")
        view = v.ImageDeleteView()
        view.kwargs = {"pk": 1}
        with mock.patch.object(v, "get_object_or_404", return_value=img), \
                mock.patch.object(v, "ImageBackend", return_value=backend), \
                mock.patch.object(image.compute_v1, "ImagesClient",
                                  side_effect=DefaultCredentialsError("gone")), \
                mock.patch.dict(image.os.environ), \
                self.assertLogs(level="WARNING"):
            response = view.post(RequestFactory().post("/"))
        self.assertEqual(response.status_code, 200)
        self.assertIs(json.loads(response.content)["success"], True)
        img.delete.assert_called_once()


class ImageDeleteViewKeepsTheRecordOnFailureTests(SimpleTestCase):
    def _post(self, deleted, source_image_project="debian-cloud",
              cmek_enabled=False):
        from ..views import images as v
        img = mock.MagicMock(source_image_project=source_image_project,
                             cmek_enabled=cmek_enabled, status_message="")
        view = v.ImageDeleteView()
        view.kwargs = {"pk": 1}
        with mock.patch.object(v, "get_object_or_404", return_value=img), \
                mock.patch.object(v, "ImageBackend") as backend:
            backend.return_value.delete_image.return_value = deleted
            response = view.post(RequestFactory().post("/"))
        self.backend = backend
        return img, json.loads(response.content)

    def test_an_encrypted_image_typed_as_imported_tears_down_its_key(self):
        img, body = self._post(True, "Imported", cmek_enabled=True)
        self.backend.return_value.delete_image.assert_called_once()
        img.delete.assert_called_once()
        self.assertEqual(body, {"success": True})

    def test_a_real_import_only_deletes_the_row(self):
        img, body = self._post(True, "Imported")
        self.backend.assert_not_called()
        img.delete.assert_called_once()
        self.assertEqual(body, {"success": True, "import": True})

    def test_a_missing_image_is_a_404(self):
        from django.db.models.query import QuerySet
        from django.http import Http404
        from ..views import images as v
        view = v.ImageDeleteView()
        view.kwargs = {"pk": 1}
        with mock.patch.object(QuerySet, "get",
                               side_effect=v.Image.DoesNotExist), \
                self.assertRaises(Http404):
            view.post(RequestFactory().post("/"))

    def test_a_failed_teardown_keeps_the_row_and_says_why(self):
        img, body = self._post(False)
        img.delete.assert_not_called()
        self.assertIs(body["success"], False)
        self.assertIn("kept", body["error"])
        self.assertEqual(img.status_message, body["error"])
        img.save.assert_called_once()

    def test_a_clean_teardown_deletes_the_row(self):
        img, body = self._post(True)
        img.delete.assert_called_once()
        self.assertEqual(body, {"success": True})

    def test_the_list_page_alerts_on_a_failed_delete(self):
        script = (ROOT / "templates/image/list.html").read_text()
        failed = script.index("response.success === false")
        self.assertIn("window.alert(response.error",
                      script[failed:failed + 200])


class KeyGenerationFailureStopsTheDeployTests(SimpleTestCase):
    """A blueprint (key) generation error must not be logged and skipped."""

    def _info(self):
        from ..cluster_manager import clusterinfo
        info = clusterinfo.ClusterInfo.__new__(clusterinfo.ClusterInfo)
        info.cluster_dir = Path(tempfile.mkdtemp())
        info.cluster = SimpleNamespace(
            name="c1", status="n", cloud_state="", cloud_credential=CREDENTIAL,
            save=lambda **kwargs: None)
        return info

    def test_prepare_raises_and_keeps_the_previous_blueprint(self):
        info = self._info()
        blueprint = info.cluster_dir / "cluster.yaml"
        blueprint.write_text("previous")
        with mock.patch.object(
                info, "_cmek_modules_yaml",
                side_effect=cmek.CmekConfigError("no key")), \
                self.assertLogs(level="ERROR") as logs, \
                self.assertRaises(cmek.CmekConfigError):
            info._prepare_ghpc_yaml()
        self.assertEqual(blueprint.read_text(), "previous")
        self.assertIn("creating blueprint for cluster c1", logs.output[0])

    def test_start_marks_the_cluster_failed(self):
        info = self._info()
        with mock.patch.object(info, "_create_cluster_dir"), \
                mock.patch.object(info, "_set_credentials"), \
                mock.patch.object(info, "_run_ghpc") as run_ghpc, \
                mock.patch.object(
                    info, "update",
                    side_effect=cmek.CmekConfigError("no key")), \
                self.assertRaises(cmek.CmekConfigError):
            info.start_cluster("{}")
        run_ghpc.assert_not_called()
        self.assertEqual(info.cluster.status, "e")
        self.assertEqual(info.cluster.cloud_state, "nm")

    def test_reconfigure_records_why_and_leaves_the_cluster_as_it_was(self):
        from ..views import clusters as v
        cluster = mock.MagicMock(status="r", cloud_state="m",
                                 status_message="")
        with mock.patch.object(v, "ClusterInfo") as info, \
                self.assertRaises(cmek.CmekConfigError):
            info.return_value.update.side_effect = cmek.CmekConfigError(
                "no key")
            v.BackendReconfigureCluster().cmd(None, None, cluster)
        info.return_value.reconfigure_cluster.assert_not_called()
        self.assertIn("no key", cluster.status_message)
        self.assertEqual((cluster.status, cluster.cloud_state), ("r", "m"))
