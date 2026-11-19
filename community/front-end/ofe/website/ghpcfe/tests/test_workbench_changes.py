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

"""Source-level checks on the Workbench form, templates and Terraform.

Workbench Instances always use the default Workbench image, so an image-family
choice in the portal would be silently ignored: it must stay out of the form,
the templates and the generated tfvars, and Terraform must keep rejecting it.
The data disk size must come from a variable, not a hardcoded number.

These read files as text, or run the real startup-script writer with
stand-in objects, so they need neither Django nor Terraform.
"""

import importlib.util
import pathlib
import re
import sys
import types
from unittest import mock

OFE = pathlib.Path(__file__).resolve().parents[3]
GHPCFE = OFE / "website" / "ghpcfe"
TF_DIR = OFE / "infrastructure_files" / "workbench_tf" / "google"


def _workbench_form_blocks():
    src = (GHPCFE / "forms.py").read_text()
    # Slice the Workbench form class(es) so other forms' fields are ignored.
    blocks = re.findall(
        r"^class \w*Workbench\w*\(.*?(?=^class |\Z)", src, re.M | re.S
    )
    assert blocks, "no Workbench form classes found"
    return blocks


def test_workbench_forms_do_not_expose_image_family():
    for block in _workbench_form_blocks():
        assert "image_family" not in block


def test_workbench_templates_do_not_show_image_family():
    templates = list((GHPCFE / "templates" / "workbench").glob("*.html"))
    assert templates
    for path in templates:
        assert "image_family" not in path.read_text(), path.name


def test_workbench_tfvars_do_not_write_image_family():
    src = (GHPCFE / "cluster_manager" / "workbenchinfo.py").read_text()
    assert "image_family" not in src


def test_terraform_still_rejects_image_family():
    src = (TF_DIR / "variables.tf").read_text()
    assert re.search(r"condition\s*=\s*var\.image_family == null", src)


def test_data_disk_size_is_a_variable():
    main = (TF_DIR / "main.tf").read_text()
    block = re.search(r"data_disks\s*\{(.*?)\n\s*\}", main, re.S).group(1)
    assert re.search(r"disk_size_gb\s*=\s*var\.data_disk_size_gb\b", block)
    assert not re.search(r"disk_size_gb\s*=\s*\d+", block)
    variables = (TF_DIR / "variables.tf").read_text()
    assert 'variable "data_disk_size_gb"' in variables


def _load_workbenchinfo():
    """Import cluster_manager/workbenchinfo.py without Django or its packages.

    copy_startup_script uses neither `utils` nor `retry`, so both are stubbed,
    and the module goes in a throwaway package so `from . import utils`
    resolves to the stub.
    """
    pkg = types.ModuleType("wb_stub")
    pkg.__path__ = []
    pkg.utils = types.ModuleType("wb_stub.utils")
    retry_mod = types.ModuleType("retry")
    retry_mod.retry = lambda *args, **kwargs: (lambda func: func)
    stubs = {"wb_stub": pkg, "wb_stub.utils": pkg.utils, "retry": retry_mod}
    with mock.patch.dict(sys.modules, stubs):
        spec = importlib.util.spec_from_file_location(
            "wb_stub.workbenchinfo", GHPCFE / "cluster_manager" / "workbenchinfo.py"
        )
        module = importlib.util.module_from_spec(spec)
        sys.modules[spec.name] = module
        spec.loader.exec_module(module)
    return module


def _render_startup_script(tmp_path, social_account):
    wbinfo = _load_workbenchinfo()
    template_dir = tmp_path / "infrastructure_files" / "gcs_bucket" / "workbench"
    template_dir.mkdir(parents=True)
    (template_dir / "startup_script_template.sh").write_text("# template\n")
    user = mock.Mock()
    user.socialaccount_set.first.return_value = social_account
    workbench = mock.Mock(id=1, trusted_user=user, attached_cluster=None)
    workbench.mount_points.order_by.return_value = []
    info = wbinfo.WorkbenchInfo.__new__(wbinfo.WorkbenchInfo)
    info.config = {"baseDir": tmp_path}
    info.workbench = workbench
    info.workbench_dir = tmp_path
    info.copy_startup_script()
    return (tmp_path / "startup_script.sh").read_text()


def test_startup_script_without_a_linked_google_account(tmp_path):
    # The OS Login name is only logged, so a trusted user with no social
    # account must still get a startup script.
    script = _render_startup_script(tmp_path, None)
    assert 'select ( .name == "")' in script
    assert "oslogin user resolved to:" in script


def test_startup_script_looks_up_the_linked_google_account(tmp_path):
    script = _render_startup_script(tmp_path, mock.Mock(uid="1234567890"))
    assert 'select ( .name == "1234567890")' in script
