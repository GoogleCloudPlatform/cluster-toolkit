#!/usr/bin/env python3
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
"""
tools/cleanup-triage-bucket.py

This tool sweeps the Triage GCS bucket and deletes any build artifacts 
older than 5 days. It explicitly protects root-level configuration files.
"""

import argparse
import datetime
from google.cloud import storage
from google.api_core.exceptions import NotFound, Forbidden

# Files in the root of the bucket that should NEVER be deleted
PROTECTED_FILES = {
    "config_triage_agent.env",
    "state.json",
    "error_categories.json",
}

def cleanup_bucket(project_id: str, bucket_name: str, days_to_keep: int):
    client = storage.Client(project=project_id)
    bucket = client.bucket(bucket_name)
    
    # Calculate cutoff date in UTC
    now = datetime.datetime.now(datetime.timezone.utc)
    cutoff_date = now - datetime.timedelta(days=days_to_keep)
    
    print(f"Starting cleanup for bucket: {bucket_name}")
    print(f"Deleting files older than: {cutoff_date.isoformat()}")

    blobs = bucket.list_blobs()

    deleted_count = 0

    try:
        for blob in blobs:
            # Protect root-level configuration files
            if blob.name in PROTECTED_FILES:
                continue
                
            if blob.time_created and blob.time_created < cutoff_date:
                try:
                    print(f"Deleting: {blob.name}")
                    blob.delete()
                    deleted_count += 1
                except NotFound:
                    print(f"Skipping {blob.name}: Already deleted concurrently.")
                except Forbidden:
                    print(f"Failed to delete {blob.name}: Permission denied (Forbidden).")
                except Exception as e:
                    print(f"Failed to delete {blob.name}: {e}")
    except NotFound:
        print(f"CRITICAL: Bucket {bucket_name} was not found.")
        raise
    except Forbidden:
        print(f"CRITICAL: Permission denied (Forbidden) accessing bucket {bucket_name}. Check IAM roles.")
        raise
    except Exception as e:
        print(f"Error iterating blobs in {bucket_name}: {e}")
        raise

    print(f"Cleanup complete. Deleted {deleted_count} files.")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Clean up old triage builds in GCS.")
    parser.add_argument("--project_id", type=str, required=True, help="Google Cloud project ID")
    parser.add_argument("--bucket_name", type=str, required=True, help="Triage GCS bucket name")
    parser.add_argument("--days", type=int, default=5, help="Days to retain files")
    
    args = parser.parse_args()
    cleanup_bucket(args.project_id, args.bucket_name, args.days)
