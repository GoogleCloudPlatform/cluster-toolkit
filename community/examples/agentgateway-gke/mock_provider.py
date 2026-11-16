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

"""CI-only OpenAI-compatible backend that requires a synthetic secret."""

from http.server import BaseHTTPRequestHandler, HTTPServer
import json
import time


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_GET(self):
        self.send_response(200)
        self.end_headers()

    def do_POST(self):
        if self.headers.get("Authorization") != "Bearer synthetic-test-key":
            self.send_response(401)
            self.end_headers()
            return
        if self.path != "/v1/chat/completions":
            self.send_response(404)
            self.end_headers()
            return
        request = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream" if request.get("stream") else "application/json")
        self.end_headers()
        common = {"id": "mock-completion", "model": "mock", "created": 1}
        if not request.get("stream"):
            self.wfile.write(json.dumps(dict(common, object="chat.completion", choices=[{
                "index": 0, "message": {"role": "assistant", "content": "one two"}, "finish_reason": "stop",
            }])).encode())
            return
        for content in ("one ", "two", None):
            event = dict(common, object="chat.completion.chunk", choices=[{
                "index": 0, "delta": {"content": content} if content else {},
                "finish_reason": None if content else "stop",
            }])
            self.wfile.write(("data: " + json.dumps(event) + "\n\n").encode())
            self.wfile.flush()
            time.sleep(0.1)
        self.wfile.write(b"data: [DONE]\n\n")


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
