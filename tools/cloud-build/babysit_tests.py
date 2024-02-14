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
from typing import Sequence, Dict, Callable
from dataclasses import dataclass
from google.cloud.devtools import cloudbuild_v1
from google.cloud.devtools.cloudbuild_v1.types.cloudbuild import Build, ApproveBuildRequest, ApprovalResult, RetryBuildRequest
import time
import argparse
import subprocess

DESCRIPTION = """
babysit_tests is a tool to approve & retry CloudBuild tests.
It monitors status of builds referenced by PR commit SHA,
it will approve and retry tests according to configured concurrency and retry policies.
The tool will terminate itself once there is no more actions to take or no reasons to wait for status changes.
The subset of tests to monitor can be configured by using test_selectors, e.g. "all", exact_name_of_test.
Usage:
tools/cloud-build/babysit_tests.py fafa333 all
"""

Selector = Callable[[Build], bool]
Status = Build.Status


@dataclass
class BuildAndCount:
    build: Build  # latest build for this trigger
    count: int  # total count of builds for this trigger


def selector_by_name(names: Sequence[str]) -> Selector:
    def selector(build: Build) -> bool:
        return any(trig_name(build) == n for n in names)
    return selector


SELECTORS: Dict[str, Selector] = {
    "all": lambda _: True,
    "batch": selector_by_name([
        "PR-test-batch-mpi",
        "PR-test-cloud-batch",
    ]),
    "crd": selector_by_name([
        "PR-test-chrome-remote-desktop",
        "PR-test-hpc-slurm-chromedesktop",
    ]),
    "gke": selector_by_name([
        "PR-test-gke",
        "PR-test-gke-storage",
    ]),
    "pr_legacy": selector_by_name([
        "PR-legacy-test-integration-group-1",
        "PR-legacy-test-integration-group-2",
        "PR-legacy-test-integration-group-3",
        "PR-legacy-test-integration-group-4",
        "PR-legacy-test-integration-group-5",
    ]),
    "slurm5": selector_by_name([
        "PR-test-hpc-high-io-v5",
        "PR-test-slurm-gcp-v5-hpc-centos7",
        "PR-test-slurm-gcp-v5-startup-scripts",
        "PR-test-slurm-gcp-v5-ubuntu2004",
        "PR-test-hpc-enterprise-slurm",
        "PR-test-hpc-slurm-chromedesktop",
        "PR-test-lustre-slurm",
    ]),
    "slurm6": selector_by_name([
        "PR-test-slurm-gcp-v6-tpu",
    ]),
    "spack": selector_by_name([
        "PR-test-batch-mpi",
        "PR-test-spack-gromacs",
    ]),
    "vm": selector_by_name([
        "PR-test-lustre-vm",
    ]),
}


def make_selector(t: str) -> Selector:
    return SELECTORS.get(t, selector_by_name([t]))


def trig_name(build: Build) -> str:
    return build.substitutions.get("TRIGGER_NAME", "???")


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


class UI:
    def __init__(self) -> None:
        self._status: Dict[str, Status] = {}
        self._change = False

    def on_init(self, builds: Sequence[Build]) -> None:
        for b in builds:
            self._status[b.id] = b.status
        if not builds:
            print(f"found no builds")
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
        else:
            return f"{self._render_status(build.status)}\t{trig_name(build)}\t{build.log_url}"

    def _render_status(self, status: Status) -> str:
        if status is None:
            return "NONE"
        return status.name


class Babysitter:
    def __init__(self, ui: UI,
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

        not_running = [b for b in active if b.status not in (
            Status.QUEUED, Status.WORKING)]
        num_running = len(active) - len(not_running)

        if num_running == len(active):
            return True  # waiting for results
        if num_running >= self.concurrency:
            return True  # waiting for "opening"

        pend = next(
            (b for b in not_running if b.status == Status.PENDING), None)
        if pend is not None:  # approve one of pending builds
            self._approve(pend)
            return True

        assert not_running  # sanity check
        failed = not_running[0]
        assert failed.status in [
            Status.FAILURE, Status.INTERNAL_ERROR, Status.TIMEOUT]  # sanity check
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


def get_default_project():
    res = subprocess.run(["gcloud", "config", "get-value",
                         "project"], stdout=subprocess.PIPE)
    assert res.returncode == 0
    return res.stdout.decode('ascii').strip()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=DESCRIPTION)
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
        print(f"Using project={project}")
    else:
        project = args.project
    cb = cloudbuild_v1.services.cloud_build.CloudBuildClient()
    selectors = [make_selector(s) for s in args.test_selector]
    ui = UI()
    Babysitter(ui, cb, project, args.pr_sha, selectors, args.c, args.r).do()
