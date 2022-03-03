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


# To create a cluster, we need:
# 1) Know which Cloud Provider & region/zone/project
# 2) Know authentication credentials
# 3) Know an "ID Number" or name - for directory to store state info

# 1 - Supplied via commandline
# 2 - Supplied via... Env vars / commandline?
# 3 - Supplied via commandline



import argparse
import sys

from . import utils
from .workbenchinfo import WorkbenchInfo


def update_workbench_terraform(workbench):
    wi = WorkbenchInfo(workbench)
    wi.prepare_terraform_vars()
    return 0

def create_workbench(workbench, token, credentials=None):
    utils.load_config(accessKey=token) # TODO:  allow custom path?

    workbench_info = WorkbenchInfo(workbench)

    # workbench files being created
    workbench_info.create_workbench_dir(credentials)

    return 0
