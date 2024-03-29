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

import argparse
import subprocess
import requests

from .core import Babysitter, Selector, cloudbuild_v1, trig_name
from .ui import UI

DESCRIPTION = """
babysit_tests is a tool to approve & retry CloudBuild tests.
It monitors status of builds referenced by PR commit SHA,
it will approve and retry tests according to configured concurrency and retry policies.
The tool will terminate itself once there is no more actions to take or no reasons to wait for status changes.
The subset of tests to monitor can be configured by using test selectors --tags, --names, --auto, and --all.
Usage:
$ tools/cloud-build/babysit_tests.py --pr 123 --auto

$ tools/cloud-build/babysit_tests.py --pr 123 --tags slurm5 slurm6
"""


def selector_by_name(name: str) -> Selector:
    return lambda b: trig_name(b) == name

def selector_by_tag(tag: str) -> Selector:
    return lambda b: tag in b.tags

def get_default_project():
    res = subprocess.run(["gcloud", "config", "get-value",
                         "project"], stdout=subprocess.PIPE)
    assert res.returncode == 0
    return res.stdout.decode('ascii').strip()

def get_pr(pr_num: int) -> dict:
    resp = requests.get(f"https://api.github.com/repos/GoogleCloudPlatform/hpc-toolkit/pulls/{pr_num}")
    resp.raise_for_status()
    return resp.json()

def get_changed_files_tags(base: str, head: str) -> set[str]:
    res = subprocess.run(["git", "log", f"{base}..{head}", "--name-only", "--format="], stdout=subprocess.PIPE)
    assert res.returncode == 0, "Is your local repo up to date?"
    changed_files = res.stdout.decode('ascii').strip().split("\n")
    tags = set()
    for f in changed_files:
        if f.startswith("community/"): f = f[len("community/"):]
        if not f.startswith("modules/"): continue
        parts = f.split("/")
        if len(parts) < 3: continue
        tags.add(f"m.{parts[2]}")
    return tags


def run_from_cli():
    parser = argparse.ArgumentParser(description=DESCRIPTION)
    parser.add_argument("--sha", type=str, help="Short SHA of target PR")
    parser.add_argument("--pr", type=int, help="PR number")

    parser.add_argument("--names", nargs="*", type=str, help="Match tests by exact name")
    parser.add_argument("--tags", nargs="*", type=str, help="Filter tests by tags")
    parser.add_argument("--auto", action="store_true", help="If true, will inspect changed files and run tests for them")
    parser.add_argument("--all", action="store_true", help="Run all tests")

    parser.add_argument("--project", type=str,
                        help="GCP ProjectID, if not set will use default one (`gcloud config get-value project`)")
    parser.add_argument("-c", type=int, default=1,
                        help="Number of tests to run concurrently, default is 1")
    parser.add_argument("-r", type=int, default=1,
                        help="Number of retries, to disable retries set to 0, default is 1")

    parser.add_argument("--base", type=str, help="Revision to inspect diff from")
    parser.add_argument("--head", type=str, help="Revision to inspect diff to, may be different in case of merged PRs")

    args = parser.parse_args()

    assert (args.sha is None) ^ (args.pr is None), "either --pr or --sha are required"
    if args.pr:
        pr = get_pr(args.pr)
        print(f"Using PR#{args.pr}: {pr['title']}")
        sha = pr["head"]["sha"]

        if pr["merged"]:
            print("PR is already merged")
            if args.head is None:
                # use merge commit as head, since original PR sha may not be available in Git history.
                args.head = pr["merge_commit_sha"]

        if args.base is None:
            args.base = pr["base"]["sha"]
    else:
        sha = args.sha

    if args.head is None:
        args.head = sha

    if args.project is None:
        project = get_default_project()
        print(f"Using project={project}")
    else:
        project = args.project

    selectors = []
    selectors += [selector_by_tag(t) for t in args.tags or []]
    selectors += [selector_by_name(n) for n in args.names or []]
    if args.all:
        selectors.append(lambda _: True)
    if args.auto:
        assert args.base is not None, "--base & [--head] or --pr are required for auto mode"
        auto_tags = get_changed_files_tags(args.base, args.head)
        selectors += [selector_by_tag(t) for t in auto_tags]

    ui = UI()
    cb = cloudbuild_v1.services.cloud_build.CloudBuildClient()
    Babysitter(ui, cb, project, sha, selectors, args.c, args.r).do()
