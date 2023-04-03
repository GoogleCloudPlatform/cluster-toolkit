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

from google.cloud.devtools import cloudbuild_v1
from google.cloud.devtools.cloudbuild_v1.types.cloudbuild import Build, ApproveBuildRequest, ApprovalResult
import time
import argparse
import subprocess


def trig_name(build):
    return build.substitutions.get("TRIGGER_NAME", "???")


def by_name(names):
    def selector(build):
        return any(trig_name(build) == n for n in names)
    return selector


SELECTORS = {
    "all": lambda _: True
}


def get_builds(cb, project, sha):
    req = cloudbuild_v1.ListBuildsRequest(
        project_id=project,
        filter=f"substitutions.SHORT_SHA={sha}",
        page_size=1000,
    )
    return cb.list_builds(req).builds


def render_status(status):
    if status is None:
        return "NONE"
    return status.name


def render_build(build):
    return f"{render_status(build.status)} {trig_name(build)}\t{build.log_url}"


def retrive_builds(cb, project, sha, selectors):
    builds = get_builds(cb, project, sha)
    builds = [b for b in builds if any(s(b) for s in selectors)]
    # Gather latest builds by trigger
    byt = {}
    for b in builds:
        t = trig_name(b)
        if t not in byt:
            byt[t] = b
        if b.create_time > byt[t].create_time:
            byt[t] = b
    return byt


def print_line(line):
    print("", end="\r")  # remove "dots"
    print(line)


def action(cb, builds, concurency):
    assert concurency > 0
    if all(b.status == Build.Status.SUCCESS for b in builds):
        print_line("All builds are green! We're done")
        return False
    nxt = next((b for b in builds if b.status == Build.Status.PENDING), None)
    if nxt is None:
        return True  # Nothing to do, waiting for user to re-run
    running = [b for b in builds if b.status in [
        Build.Status.WORKING, Build.Status.QUEUED]]
    if len(running) >= concurency:
        return True  # Nothing to do, waiting for "openning"
    req = ApproveBuildRequest(
        name=f"projects/{nxt.project_id}/builds/{nxt.id}",
        approval_result=ApprovalResult(
            decision=ApprovalResult.Decision.APPROVED
        )
    )
    cb.approve_build(request=req)
    return True


def do(project, sha, selectors, concurency=1):
    cb = cloudbuild_v1.services.cloud_build.CloudBuildClient()
    selectors = [SELECTORS.get(s, by_name([s])) for s in selectors]

    builds = retrive_builds(cb, project, sha, selectors)
    if not builds:
        print(f"Found no builds referencing {sha=}")
        return
    print(f"Found {len(builds)} builds:")
    for _, b in sorted(builds.items()):
        print(render_build(b))

    build_status = {t: b.status for t, b in builds.items()}
    while action(cb, builds.values(), concurency):
        builds = retrive_builds(cb, sha, selectors)

        for t, b in builds.items():
            if b.status != build_status.get(t):
                print_line(
                    f"{render_status(build_status.get(t))}>{render_build(b)}")
                build_status[t] = b.status
        print(".", end="", flush=True)
        time.sleep(10)


def get_default_project():
    res = subprocess.run(["gcloud", "config", "get-value", "project"], stdout=subprocess.PIPE)
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
    args = parser.parse_args()
    if args.project is None:
        project = get_default_project()
        print(f"Using {project=}")
    else:
        project = args.project
    do(project, args.pr_sha, args.test_selector, concurency=args.c)
