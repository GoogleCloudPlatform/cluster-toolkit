#!/usr/bin/env python3
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

"""Helper functions for workbench lifecycle management"""

from .utils import run_terraform, load_config
from .workbenchinfo import WorkbenchInfo


def update_workbench(workbench):
    wi = WorkbenchInfo(workbench)
    wi.copy_startup_script()
    return 0


def create_workbench(workbench, token, credentials=None):
    load_config(access_key=token)  # TODO:  allow custom path?

    workbench_info = WorkbenchInfo(workbench)

    # workbench files being created
    workbench_info.create_workbench_dir(credentials)

    return 0


def start_workbench(workbench, token):
    workbench.cloud_state = "nm"
    workbench.status = "c"
    workbench.save()
    load_config(access_key=token)  # TODO:  allow custom path?
    wi = WorkbenchInfo(workbench)
    wi.initialize_terraform()
    wi.run_terraform()
    workbench.cloud_state = "m"
    workbench.save()


def destroy_workbench(workbench, unused_token):
    """
    Destroy a workbench.

        Parameters:
            args - an object with the following members:
                'workbench_id' - id # of the compute workbench
                'access_key' - DB access key
    """

    wb = WorkbenchInfo(workbench)
    extra_env = {
        "GOOGLE_APPLICATION_CREDENTIALS": wb._get_credentials_file()  # pylint: disable=protected-access
    }

    config = load_config()

    workbench.status = "t"
    workbench.cloud_state = "dm"
    workbench.save()
    workbench_dir = (
        config["baseDir"] / "workbenches" / f"workbench_{workbench.id}"
    )

    run_terraform(
        workbench_dir / "terraform" / "google", "destroy", extra_env=extra_env
    )
    workbench.status = "d"
    workbench.cloud_state = "xm"
    workbench.save()
