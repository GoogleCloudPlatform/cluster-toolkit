# Copyright 2026 Google LLC
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

from typing import Optional, Sequence, Union

from IPython.core.display import display, HTML, clear_output
from datetime import datetime
import pytz

from .cli_ui import CliUI
from .core import Build, Status, latest_by_trigger, trig_name

class NotebookUI(CliUI):
    def __init__(self) -> None:
        super().__init__()
        self.log: list[Union[HTML, str]] = []
        self.tz = pytz.timezone('America/Los_Angeles')
  
    def on_update(self, builds: Sequence[Build]) -> None:
        redraw = False
        for b in builds:
            if b.status != self._status.get(b.id):
                br = self._render_build(b)
                sr = self._render_status(self._status.get(b.id))
                self.log.append(HTML(f"{self._now()} status update: {sr} > {br}"))
                self._change = True
                redraw = True
            self._status[b.id] = b.status
        if redraw:
            self._render_summary(builds)

    def on_action(self, action: str, build: Build) -> None:
        self.log.append(HTML(f"{self._now()} {action} {self._render_build(build)}"))
        
    def _render_summary(self, builds: Sequence[Build]) -> None:
        clear_output(wait=True)
        for _, bc in sorted(latest_by_trigger(builds).items()):
            display(HTML(self._render_build(bc.build, bc.count)))
        display(HTML("<hr>"))
        for l in reversed(self.log):
            display(l)
        
    
    def _render_build(self, build: Build, count=1) -> str:
        link = f"<a href='{build.log_url}'>{trig_name(build)}</a>"
        if count > 1:
            return f"{self._render_status(build.status)}\t{link}(try #{count})"
        else:
            return f"{self._render_status(build.status)}\t{link}"

    def _render_status(self, status: Optional[Status]) -> str:   
        marks = {
            Status.QUEUED: "🔵",
            Status.WORKING: "🔵",
            Status.FAILURE: "🔴",
            Status.INTERNAL_ERROR: "🔴",
            Status.TIMEOUT: "🔴",
            Status.CANCELLED:  "🔴", 
            Status.EXPIRED:  "🔴",
            Status.SUCCESS: "🟢"}
        mark = marks.get(status, "⚪")
        name = "new" if not status else status.name.lower()
        return f"{mark} {name}"
    
    def _now(self) -> str:
        return datetime.now(self.tz).strftime("%H:%M:%S")
