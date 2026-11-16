# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     https://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Wait for the synthetic provider's Service to accept HTTP before testing routes."""

import argparse
import time
import urllib.error
import urllib.request


def wait_for_http(url, timeout=120, interval=2):
    deadline = time.monotonic() + timeout
    last_error = None
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url, timeout=min(5, max(0.001, deadline - time.monotonic()))) as response:
                if response.status == 200:
                    print("Synthetic provider Service is reachable", flush=True)
                    return
                last_error = f"HTTP {response.status}"
        except urllib.error.HTTPError as error:
            if error.code not in (404, 429, 500, 502, 503, 504):
                error.close()
                raise
            last_error = f"HTTP {error.code}"
            error.close()
        except (urllib.error.URLError, TimeoutError) as error:
            last_error = str(error)
        time.sleep(min(interval, max(0, deadline - time.monotonic())))
    raise TimeoutError(f"Synthetic provider Service did not become reachable: {last_error}")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("url")
    args = parser.parse_args()
    wait_for_http(args.url)
