# Copyright 2022 Google LLC
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

"""Spack package operations"""

from . import utils
import sys

spack_prefix = utils.g_baseDir / "dependencies" / "spack"
spack_path_lib = spack_prefix / "lib" / "spack"
spack_external_libs = spack_path_lib / "external"

sys.path.insert(0, spack_path_lib.as_posix())
sys.path.insert(0, spack_external_libs.as_posix())

#pylint: disable=wrong-import-position
import spack.main
import spack.repo
import spack.version
#pylint: enable=wrong-import-position


def get_package_list():
    return spack.repo.all_package_names()


def get_package_info(names):
    pkgs = [spack.repo.PATH.get_pkg_class(name) for name in names]
    return (
        {
            "name": pkg.name,
            "latest_version": str(
                spack.version.VersionList(pkg.versions).preferred()
            ),
            "versions": [str(v) for v in reversed(sorted(pkg.versions))],
            "variants": [k for k, v in pkg.variants.items()],
            "description": pkg.format_doc(),
        }
        for pkg in pkgs
    )
