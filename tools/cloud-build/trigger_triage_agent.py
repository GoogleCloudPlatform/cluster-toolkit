#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import sys
import json
import subprocess
import urllib.request
import urllib.error

def main():
    """
    Triggers the asynchronous Cloud Run Triage Agent via an authenticated REST POST request.
    
    Required Environment Variables:
    - TRIAGE_BUILD_ID: Unique string identifying the active build execution.
    - TRIAGE_INVOKER_SA: Service Account email to safely impersonate for authorization.
    - TRIAGE_CLOUD_RUN_URL: Target endpoint URL for the backend Triage service.
    - TRIAGE_PROJECT_NUMBER: GCP project number associated with the active pipeline.
    """
    build_id = os.environ.get("TRIAGE_BUILD_ID")
    project_number = os.environ.get("TRIAGE_PROJECT_NUMBER")
    cloud_run_url = os.environ.get("TRIAGE_CLOUD_RUN_URL")
    invoker_sa = os.environ.get("TRIAGE_INVOKER_SA")

    if not all([build_id, project_number, cloud_run_url, invoker_sa]):
        print("Missing required environment variables for Triage Agent Trigger.", file=sys.stderr)
        sys.exit(1)

    try:
        proc = subprocess.run(
            ["gcloud", "auth", "print-identity-token", 
             "--impersonate-service-account", invoker_sa,
             "--audiences", cloud_run_url],
            capture_output=True, text=True, check=True
        )
        token = proc.stdout.strip()
    except subprocess.CalledProcessError as e:
        print(f"Failed to get identity token: {e.stderr}", file=sys.stderr)
        sys.exit(1)

    url = f"{cloud_run_url.rstrip('/')}/trigger"
    data = json.dumps({"build_id": build_id, "project_number": project_number}).encode("utf-8")
    
    req = urllib.request.Request(url, data=data, headers={
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    })

    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            if response.status != 202:
                print(f"Failed to trigger agent. HTTP Status: {response.status}", file=sys.stderr)
                print(f"Response Body: {response.read().decode()}", file=sys.stderr)
                sys.exit(1)
    except urllib.error.HTTPError as e:
        print(f"Failed to trigger agent. HTTP Status: {e.code}", file=sys.stderr)
        print(f"Response Body: {e.read().decode()}", file=sys.stderr)
        sys.exit(1)
    except urllib.error.URLError as e:
        print(f"Failed to trigger agent. Network error: {e.reason}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
