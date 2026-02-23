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

import random
from typing import Sequence, Dict, Callable, Protocol
from dataclasses import dataclass
from google.cloud.devtools import cloudbuild_v1 # pip install google-cloud-build
from google.cloud.devtools.cloudbuild_v1.types.cloudbuild import Build, ApproveBuildRequest, ApprovalResult, RetryBuildRequest


Selector = Callable[[Build], bool]
Status = Build.Status


@dataclass
class BuildAndCount:
    build: Build  # latest build for this trigger
    count: int  # total count of builds for this trigger


class UIProto(Protocol): # just an interface
    # TODO: add docs
    def on_init(self, builds: Sequence[Build]) -> None: ...
    def on_done(self, builds: Sequence[Build]) -> None: ...
    def on_update(self, builds: Sequence[Build]) -> None: ...
    def on_action(self, action: str, build: Build) -> None: ...
    def sleep(self, sec: int) -> None: ...


def latest_by_trigger(builds: Sequence[Build]) -> Dict[str, BuildAndCount]:
    """
    Returns a map trigger_name -> (latest_build, num_of_builds)
    """
    byt: Dict[str, BuildAndCount] = {}
    for b in builds:
        t = trig_name(b)
        if t not in byt:
            byt[t] = BuildAndCount(b, 0)
        if b.create_time > byt[t].build.create_time:
            byt[t].build = b
        byt[t].count += 1
    return byt

def trig_name(build: Build) -> str:
    return build.substitutions.get("TRIGGER_NAME", "???")


class Babysitter:
    def __init__(self, ui: UIProto,
                 cb: cloudbuild_v1.services.cloud_build.CloudBuildClient,
                 project: str,
                 sha: str,
                 selectors: Sequence[Selector],
                 concurrency: int,
                 retries: int) -> None:
        self.ui = ui
        self.cb = cb
        self.project = project
        self.sha = sha
        self.selectors = list(selectors)
        self.concurrency = concurrency
        self.retries = retries

    def _get_builds(self) -> Sequence[Build]:
        req = cloudbuild_v1.ListBuildsRequest(
            project_id=self.project,
            # cloud build only recognizes SHORT_SHA of length 7
            filter=f"substitutions.SHORT_SHA={self.sha[:7]}",
            page_size=1000,
        )
        builds = self.cb.list_builds(req).builds
        return [b for b in builds if any(s(b) for s in self.selectors)]

    def _in_terminal_state(self, bc: BuildAndCount) -> bool:
        if bc.build.status in [Status.STATUS_UNKNOWN, Status.CANCELLED, Status.EXPIRED, Status.SUCCESS]:
            return True

        if bc.build.status in [Status.PENDING, Status.QUEUED, Status.WORKING]:
            return False

        if bc.build.status in [Status.FAILURE, Status.INTERNAL_ERROR, Status.TIMEOUT]:
            return bc.count > self.retries
        assert False, f"Unexpected status {bc.build.status}"

    def _approve(self, build: Build) -> None:
        self.ui.on_action("approve", build)
        req = ApproveBuildRequest(
            name=f"projects/{build.project_id}/builds/{build.id}",
            approval_result=ApprovalResult(
                decision=ApprovalResult.Decision.APPROVED
            )
        )
        self.cb.approve_build(request=req)

    def _retry(self, build: Build) -> None:
        self.ui.on_action("retry", build)
        req = RetryBuildRequest(project_id=build.project_id, id=build.id)
        self.cb.retry_build(request=req)

    def _take_action(self, builds: Sequence[Build]) -> bool:
        """
        Returns bool - whether "babysitting" should be continued.
        """
        latest = latest_by_trigger(builds).values()
        active = [bc.build for bc in latest if not self._in_terminal_state(bc)]
        if not active:
            return False  # all builds are in terminal state, done

        not_running = [b for b in active if b.status not in (Status.QUEUED, Status.WORKING)]
        num_running = len(active) - len(not_running)

        if num_running == len(active):
            return True  # waiting for results
        if num_running >= self.concurrency:
            return True  # waiting for "opening"

        pending = [b for b in not_running if b.status == Status.PENDING]
        if pending:  # approve one of pending builds
            self._approve(random.choice(pending))
            return True

        assert not_running  # sanity check
        failed = random.choice(not_running)
        assert failed.status in [Status.FAILURE, Status.INTERNAL_ERROR, Status.TIMEOUT]  # sanity check
        self._retry(failed)  # retry failed build
        return True

    def do(self):
        builds = self._get_builds()
        self.ui.on_init(builds)
        if not builds:
            return

        while self._take_action(builds):
            self.ui.sleep(10)
            builds = self._get_builds()
            self.ui.on_update(builds)
        self.ui.on_done(builds)
