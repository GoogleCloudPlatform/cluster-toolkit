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
from google.cloud.devtools.cloudbuild_v1.types.cloudbuild import Build
from google.cloud import storage
from datetime import timezone, timedelta, datetime, date
import glob
import os
import re
import argparse
import subprocess

def run(project):
    cb = cloudbuild_v1.services.cloud_build.CloudBuildClient()
    req = cloudbuild_v1.ListBuildsRequest(
        project_id=project, 
        filter="status=TIMEOUT", 
        page_size=1000
    )
    builds = cb.list_builds(req).builds
    if not builds:
        print("No timedout builds found this week.")
        return

    PDT = timezone(timedelta(hours=-7))
    pastweek = datetime.combine(date.today(), datetime.min.time()).astimezone(PDT) - timedelta(days=7)
    for i, b in enumerate(builds):
        st = b.start_time.astimezone(PDT)
        if st < pastweek: continue
        tg = b.substitutions["TRIGGER_NAME"]
        print(f"[{i}]\t{st.date()}\t{b.log_url}\t{tg}")
    idx = int(input("Select build # to purge: "))
    assert 0 <= idx < len(builds), f"Invalid index {idx}"
    print(purge(builds[idx]))

def purge(build):
    storage_client = storage.Client()
    bucket = storage_client.bucket(build.logs_bucket[5:])
    logs = bucket.blob("log-%s.txt" % build.id).download_as_string().decode('utf-8')
    mtch = re.search("gs://[\w-]*tf-state/.*\.tgz", logs)
    assert mtch is not None, "can find link to tfstate"
    link = mtch.group()
    res = os.system(f"""gcloud storage cp {link} "/tmp/to_purge_tgz" && \
rm -rf "/tmp/to_purge"; mkdir "/tmp/to_purge" && \
tar zxf "/tmp/to_purge_tgz" -C "/tmp/to_purge" """)
    assert res == 0
    for d in glob.glob("/tmp/to_purge/*/*"):
        res = os.system(f"terraform -chdir={d} init && terraform -chdir={d} destroy --auto-approve")
        assert res == 0
    

def get_default_project():
    res = subprocess.run(["gcloud", "config", "get-value",
                         "project"], stdout=subprocess.PIPE)
    assert res.returncode == 0
    return res.stdout.decode('ascii').strip()

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--project", type=str,
                        help="GCP ProjectID, if not set will use default one (`gcloud config get-value project`)")
    args = parser.parse_args()
    if args.project is None:
        project = get_default_project()
        print(f"Using project={project}")
    else:
        project = args.project
   
    run(project)
