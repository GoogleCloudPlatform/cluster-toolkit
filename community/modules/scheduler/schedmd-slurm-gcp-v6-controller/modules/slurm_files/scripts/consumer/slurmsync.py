#!/slurm/python/venv/bin/python3.13

# Copyright 2025 Google LLC
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

import logging
from pathlib import Path
import time
import util
import fcntl
import sys

# !!!  logging file
log = logging.getLogger()


def reconfigure_slurm():
    pass # !!!! ? Fetch config (small one)



def main():
    pass # !!! TODO
    # try:
    #     reconfigure_slurm()
    # except Exception:
    #     log.exception("failed to reconfigure slurm")
    
    # try:
    #     # TODO: it performs 1 to 4 GCS list requests,
    #     # use cached version, combine with `_list_config_blobs`
    #     install_custom_scripts(check_hash=True)
    # except Exception:
    #     log.exception("failed to sync custom scripts")



if __name__ == "__main__":
    
    pid_file = (Path("/tmp") / Path(__file__).name).with_suffix(".pid")
    with pid_file.open("w") as fp:
        try:
            fcntl.lockf(fp, fcntl.LOCK_EX | fcntl.LOCK_NB)
            util.init_log("slurmsync")
            main()
        except BlockingIOError:
            sys.exit(0)
