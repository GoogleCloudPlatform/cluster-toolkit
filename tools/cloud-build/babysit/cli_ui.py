# Copyright 2024 Google LLC
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

from typing import Sequence, Dict, Optional
import time
from enum import Enum

from .core import Status, Build, latest_by_trigger, trig_name


class Color(Enum):
    GREEN = "\033[92m"
    YELLOW = "\033[93m"
    RED = "\033[91m"
    BLUE = "\033[94m"
    END = "\033[0m"

class CliUI: # implements UIProto
    def __init__(self, pretty=False) -> None:
        self._status: Dict[str, Status] = {}
        self._change = False
        self._pretty = pretty

    def on_init(self, builds: Sequence[Build]) -> None:
        for b in builds:
            self._status[b.id] = b.status
        if not builds:
            print("found no builds")
        else:
            print(f"found {len(builds)} builds:")
            self._render_summary(builds)

    def on_done(self, builds: Sequence[Build]) -> None:
        print("done")
        if self._change:
            self._render_summary(builds)

    def on_update(self, builds: Sequence[Build]) -> None:
        for b in builds:
            if b.status != self._status.get(b.id):
                br = self._render_build(b)
                sr = self._render_status(self._status.get(b.id))
                print(f"status update: {sr} > {br}")
                self._change = True
            self._status[b.id] = b.status

    def on_action(self, action: str, build: Build) -> None:
        print(f"{action} {self._render_build(build)}")

    def sleep(self, sec: int) -> None:
        time.sleep(sec)

    def _render_summary(self, builds: Sequence[Build]) -> None:
        order_fn = lambda bc: (bc.build.status, trig_name(bc.build))

        ordered = sorted(latest_by_trigger(builds).values(), key=order_fn)
        for bc in ordered:
            print(self._render_build(bc.build, bc.count))

    def _render_build(self, build: Build, count:int=1) -> str:
        status = self._render_status(build.status)
        cnt = f"[{count}]" if count > 1 else "   "
        link = self._render_link(build)
        return f"{status}{cnt} {link}"

    def _render_status(self, status: Optional[Status]) -> str:
        sn = "NONE" if status is None else status.name
        if not self._pretty: return sn
        CM = {
            Status.SUCCESS: Color.GREEN,
            Status.FAILURE: Color.RED,
            Status.TIMEOUT: Color.RED,
            Status.PENDING: Color.END, # default
            Status.QUEUED: Color.BLUE,
            Status.WORKING: Color.BLUE,
        }
        def_color = Color.YELLOW # render "unusual" states with something bright
        clr = CM.get(status, def_color).value
        return f"{clr}{sn}{Color.END.value}"
    
    def _render_link(self, build: Build) -> str:
        name, url = trig_name(build), build.log_url
        if not self._pretty: return f"{name}\t{url}"
        return f"\033]8;;{url}\033\\{name}\033]8;;\033\\"
