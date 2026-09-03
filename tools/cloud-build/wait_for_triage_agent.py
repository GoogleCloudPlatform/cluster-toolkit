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
import time
import json
import subprocess
import urllib.request
import urllib.error

def main():
    """
    Polls the Firestore database REST API to determine when the async Triage Agent completes.
    
    Required Environment Variables:
    - TRIAGE_BUILD_ID: Unique string identifying the active build document.
    - TRIAGE_INVOKER_SA: Service Account email to safely impersonate for authorization.
    - TRIAGE_FIRESTORE_URL: The Firestore REST API base endpoint for the diagnostics DB.
    """
    build_id = os.environ.get("TRIAGE_BUILD_ID")
    firestore_url = os.environ.get("TRIAGE_FIRESTORE_URL")
    invoker_sa = os.environ.get("TRIAGE_INVOKER_SA")

    if not all([build_id, firestore_url, invoker_sa]):
        print("Missing required environment variables for Triage Agent verification.", file=sys.stderr)
        sys.exit(1)

    try:
        proc = subprocess.run(
            ["gcloud", "auth", "print-access-token", "--impersonate-service-account", invoker_sa],
            capture_output=True, text=True, check=True
        )
        token = proc.stdout.strip()
    except subprocess.CalledProcessError as e:
        print(f"Failed to get access token: {e.stderr}", file=sys.stderr)
        sys.exit(1)

    url = f"{firestore_url.rstrip('/')}/{build_id}"
    req = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})

    # Wait for Cloud Run to dynamically hydrate the Document
    doc_hydrated = False
    for _ in range(12):
        try:
            with urllib.request.urlopen(req, timeout=10) as response:
                doc = json.loads(response.read().decode())
            if "name" in doc:
                doc_hydrated = True
                break
        except urllib.error.HTTPError as e:
            if e.code in (401, 403):
                print(f"Auth error ({e.code}): Permission denied when accessing Firestore.", file=sys.stderr)
                sys.exit(1)
        except (urllib.error.URLError, json.JSONDecodeError):
            pass
        time.sleep(5)

    if not doc_hydrated:
        print("Agent failed to start: Database document was not created within 60 seconds.", file=sys.stderr)
        sys.exit(1)

    # Poll for completion status
    for _ in range(30):
        try:
            with urllib.request.urlopen(req, timeout=10) as response:
                doc = json.loads(response.read().decode())
                
            status = doc.get("fields", {}).get("status", {}).get("stringValue", "")
            if status in ["completed", "failed"]:
                try:
                    exec_sum = (doc.get("fields", {})
                                  .get("report", {})
                                  .get("mapValue", {})
                                  .get("fields", {})
                                  .get("executive_summary", {})
                                  .get("stringValue", ""))
                except (AttributeError, TypeError):
                    exec_sum = ""
                
                print(json.dumps({"status": status, "executive_summary": exec_sum}))
                sys.exit(0)
        except urllib.error.HTTPError as e:
            if e.code in (401, 403):
                print(f"Auth error ({e.code}): Permission denied when accessing Firestore.", file=sys.stderr)
                sys.exit(1)
        except (urllib.error.URLError, json.JSONDecodeError):
            pass
        
        time.sleep(30)
        
    print("Agent failed to complete analysis within the time limit.", file=sys.stderr)
    sys.exit(1)

if __name__ == "__main__":
    main()
