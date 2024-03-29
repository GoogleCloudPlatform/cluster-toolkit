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

from .core import Status, Build, latest_by_trigger, trig_name


class UI: # implements UIProto
    def __init__(self) -> None:
        self._status: Dict[str, Status] = {}
        self._change = False

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
        for _, bc in sorted(latest_by_trigger(builds).items()):
            print(self._render_build(bc.build, bc.count))

    def _render_build(self, build: Build, count=1) -> str:
        if count > 1:
            return f"{self._render_status(build.status)}[{count}]\t{trig_name(build)}\t{build.log_url}"
        return f"{self._render_status(build.status)}\t{trig_name(build)}\t{build.log_url}"

    def _render_status(self, status: Optional[Status]) -> str:
        if status is None:
            return "NONE"
        return status.name
