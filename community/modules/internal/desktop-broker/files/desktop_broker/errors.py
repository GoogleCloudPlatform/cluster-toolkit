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

"""The one error type callers are expected to translate into a response."""


class BrokerError(Exception):
    """An error with an HTTP status and a message safe to show a user.

    Messages reach the browser, so they say what to do about the problem and
    never include tokens, secrets or internal paths.
    """

    def __init__(self, status, message):
        super().__init__(message)
        self.status = status
        self.message = message
