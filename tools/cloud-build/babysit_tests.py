#!/usr/bin/env python3
# Copyright 2023 Google LLC
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
from typing import Sequence, Dict, Tuple, Callable
from dataclasses import dataclass
from google.cloud.devtools import cloudbuild_v1
from google.cloud.devtools.cloudbuild_v1.types.cloudbuild import Build, ApproveBuildRequest, ApprovalResult,RetryBuildRequest
import time
import argparse
import subprocess

Selector = Callable[[Build], bool]


def selector_by_name(names) -> Selector:
    def selector(build):
        return any(trig_name(build) == n for n in names)
    return selector


SELECTORS: Dict[str, Selector] = {
    "all": lambda _: True
}


def make_selector(t: str) -> Selector:
    return SELECTORS.get(t, selector_by_name([t]))


def trig_name(build: Build) -> str:
    return build.substitutions.get("TRIGGER_NAME", "???")


@dataclass
class BuildAndCount:
    build: Build
    count: int


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


Status = Build.Status


class Babysitter:
    def __init__(self,
                 cb: cloudbuild_v1.services.cloud_build.CloudBuildClient,
                 project: str,
                 sha: str,
                 selectors: Sequence[Selector],
                 concurrency: int,
                 retries: int) -> None:
        self.cb = cb
        self.project = project
        self.sha = sha
        self.selectors = list(selectors)
        self.concurrency = concurrency
        self.retries = retries
        self._status: Dict[str, Status] = {}

    def _get_builds(self) -> Sequence[Build]:
        req = cloudbuild_v1.ListBuildsRequest(
            project_id=self.project,
            filter=f"substitutions.SHORT_SHA={self.sha}",
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
        assert False, f"Unexpected {bc.build.status=}"

    def _approve(self, build: Build) -> None:
        req = ApproveBuildRequest(
            name=f"projects/{build.project_id}/builds/{build.id}",
            approval_result=ApprovalResult(
                decision=ApprovalResult.Decision.APPROVED
            )
        )
        self.cb.approve_build(request=req)

    def _retry(self, build: Build) -> None:
        req = RetryBuildRequest(project_id=build.project_id, id=build.id)
        self.cb.retry_build(request=req)


    def _act(self, builds: Sequence[Build]) -> bool:
        latest = latest_by_trigger(builds).values()
        active = [bc.build for bc in latest if not self._in_terminal_state(bc)]
        if not active:
            return False  # all builds are in terminal state, done

        not_running = [b for b in active if b.status not in (
            Status.QUEUED, Status.WORKING)]
        num_running = len(active) - len(not_running)

        if num_running == len(active):
            return True  # waiting for results
        if num_running >= self.concurrency:
            return True  # waiting for "openning"

        pend = next(
            (b for b in not_running if b.status == Status.PENDING), None)
        if pend is not None:  # approve one of pending builds
            self._approve(pend)
            return True

        assert not_running # sanity check
        failed = not_running[0]
        assert failed.status in [Status.FAILURE, Status.INTERNAL_ERROR, Status.TIMEOUT]  # sanity check
        self._retry(failed) # retry failed build
        return True
        

    def _sleep(self) -> None:
        time.sleep(5)
        print(".", end="", flush=True)

    def _render_summary(self, builds: Sequence[Build]) -> None:
        for _, bc in sorted(latest_by_trigger(builds).items()):
            print(render_build(bc.build, bc.count))

    def _track_status_updates(self, builds: Sequence[Build]) -> None:
        if not self._status: # state is empty, just populate
            for b in builds:
                self._status[b.id] = b.status
            return
        
        for b in builds:
            if b.status != self._status.get(b.id):
                print_line(f"{render_status(self._status.get(b.id))} > {render_build(b)}")
                self._status[b.id] = b.status


    def do(self):
        builds = self._get_builds()
        if not builds:
            print(f"Found no builds referencing SHA {self.sha}")
            return

        print(f"Found {len(builds)} builds:")
        self._render_summary(builds)
        
        acted = False
        while self._act(builds):
            acted = True
            self._sleep()
            builds = self._get_builds()
            self._track_status_updates(builds)

        print("Done, no actions left to take.")
        if acted:
            self._render_summary(builds)


def render_status(status):
    if status is None:
        return "NONE"
    return status.name


def render_build(build, count=1):
    if count > 1:
        return f"{render_status(build.status)}[{count}]\t{trig_name(build)}\t{build.log_url}"
    else:
        return f"{render_status(build.status)}\t\t{trig_name(build)}\t{build.log_url}"


def print_line(line):
    print("", end="\r")  # remove "dots"
    print(line)


def get_default_project():
    res = subprocess.run(["gcloud", "config", "get-value",
                         "project"], stdout=subprocess.PIPE)
    assert res.returncode == 0
    return res.stdout.decode('ascii').strip()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("pr_sha", type=str, help="Short SHA of target PR")
    parser.add_argument("test_selector", nargs='+', type=str,
                        help="Selector for test, currently support 'all' and exact name match")
    parser.add_argument("--project", type=str,
                        help="GCP ProjectID, if not set will use default one (`gcloud config get-value project`)")
    parser.add_argument("-c", type=int, default=1,
                        help="Number of tests to run concurrently, default is 1")
    parser.add_argument("-r", type=int, default=1,
                        help="Number of retries, to disable retries set to 0, default is 1")
    args = parser.parse_args()
    if args.project is None:
        project = get_default_project()
        print(f"Using {project=}")
    else:
        project = args.project
    cb = cloudbuild_v1.services.cloud_build.CloudBuildClient()
    selectors = [make_selector(s) for s in args.test_selector]

    Babysitter(cb, project, args.pr_sha, selectors, args.c, args.r).do()
