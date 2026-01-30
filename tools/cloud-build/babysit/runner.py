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

from typing import Optional, Collection

import requests
from dataclasses import dataclass

from .core import Babysitter, Selector, cloudbuild_v1, trig_name, UIProto

DESCRIPTION = """
babysit_tests is a tool to approve & retry CloudBuild tests.
It monitors status of builds referenced by PR commit SHA,
it will approve and retry tests according to configured concurrency and retry policies.
The tool will terminate itself once there is no more actions to take or no reasons to wait for status changes.
The subset of tests to monitor can be configured by using test selectors --tags, --names, --auto, and --all.

Usage:
$ tools/cloud-build/babysit/run --pr #### --auto --project <project id>
"""

def selector_by_name(name: str) -> Selector:
    return lambda b: trig_name(b) == name

def selector_by_tag(tag: str) -> Selector:
    return lambda b: tag in b.tags

def get_pr(pr_num: int) -> dict:
    resp = requests.get(f"https://api.github.com/repos/GoogleCloudPlatform/hpc-toolkit/pulls/{pr_num}")
    resp.raise_for_status()
    return resp.json()

def get_pr_files(pr_num: int) -> list[str]:
    resp = requests.get(f"https://api.github.com/repos/GoogleCloudPlatform/hpc-toolkit/pulls/{pr_num}/files")
    resp.raise_for_status()
    return [f['filename'] for f in resp.json()]

def get_changed_files_tags(files: Collection[str]) -> set[str]:
    tags = set()
    for f in files:
        if f.startswith("community/"): f = f[len("community/"):]
        if not f.startswith("modules/"): continue
        parts = f.split("/")
        if len(parts) < 3: continue
        tags.add(f"m.{parts[2]}")
    return tags

@dataclass
class RunnerArgs:
    pr: int
    # Test selectors, at least one is required
    names: Optional[list[str]] = None
    tags: Optional[list[str]] = None
    auto: Optional[bool] = None
    all: Optional[bool] = None
    
    # Optional project, if not set will use default one
    project: Optional[str] = None
    
    concurrency: int = 1
    retries: int = 1


def run(args: RunnerArgs, ui: UIProto) -> None:
    assert args.names or args.tags or args.auto or args.all, "At least one test selector is required"
    pr = get_pr(args.pr)
    print(f"Using PR#{args.pr}: {pr['title']}") # TODO: use UI to log
    sha = pr["head"]["sha"]
    
    selectors = []
    selectors += [selector_by_tag(t) for t in args.tags or []]
    selectors += [selector_by_name(n) for n in args.names or []]
    if args.all:
        selectors.append(lambda _: True)
    if args.auto:
        auto_tags = get_changed_files_tags(get_pr_files(args.pr))
        selectors += [selector_by_tag(t) for t in auto_tags]
    if not selectors:
        print("No test selectors found, nothing to do.") # TODO: use UI to log
        return

    cb = cloudbuild_v1.services.cloud_build.CloudBuildClient()
    try:
        Babysitter(ui, cb, args.project, sha, selectors, args.concurrency, args.retries).do()
    except KeyboardInterrupt:
        print("User interrupted") # TODO: use UI to log

def run_from_notebook(
        pr: int,
        auto: Optional[bool] = None,
        all: Optional[bool] = None,
        names: Optional[list[str]] = None,
        tags: Optional[list[str]] = None,
        concurrency: int = 1,
        retries: int = 1,
        project: str = "hpc-toolkit-dev"):
    
    args = RunnerArgs(pr, auto=auto, all=all, names=names, tags=tags, project=project, concurrency=concurrency, retries=retries)
    from .notebook_ui import NotebookUI
    run(args, NotebookUI())

def run_from_cli():
    import argparse
    from .cli_ui import CliUI

    parser = argparse.ArgumentParser(description=DESCRIPTION)
    parser.add_argument("--pr", type=int, required=True, help="PR number")

    parser.add_argument("--names", nargs="*", type=str, help="Match tests by exact name")
    parser.add_argument("--tags", nargs="*", type=str, help="Filter tests by tags")
    parser.add_argument("--auto", action="store_true", help="If true, will inspect changed files and run tests for them")
    parser.add_argument("--all", action="store_true", help="Run all tests")

    parser.add_argument("--project", type=str, default="hpc-toolkit-dev", help="GCP ProjectID")
    parser.add_argument("-c", "--concurrency", type=int, default=1,
                        help="Number of tests to run concurrently, default is 1")
    parser.add_argument("-r", "--retries", type=int, default=1,
                        help="Number of retries, to disable retries set to 0, default is 1")
    # Non-runner args
    parser.add_argument("--nocolor", action="store_true", help="Do not use color in output")

    cli_args = vars(parser.parse_args())
    short_url = cli_args.get("project") == "hpc-toolkit-dev"
    ui = CliUI(no_color=cli_args.pop("nocolor"), short_url=short_url)

    run(RunnerArgs(**cli_args), ui)
