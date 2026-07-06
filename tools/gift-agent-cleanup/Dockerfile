# Copyright 2026 "Google LLC"
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

FROM python:3.11-slim

# Install the required GCP library
RUN pip install --no-cache-dir google-cloud-storage

# Set the working directory
WORKDIR /app

# Copy the script into the container
COPY cleanup_triage_bucket.py .
RUN chmod +x cleanup_triage_bucket.py

# The entrypoint expects arguments to be passed at runtime
ENTRYPOINT ["python3", "cleanup_triage_bucket.py"]
