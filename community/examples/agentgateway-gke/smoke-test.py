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

"""Exercise the OpenAI-compatible endpoint without logging prompts or responses."""

import argparse
import json
import time
import urllib.error
import urllib.request


def check_stream(response):
    """Reject empty, truncated, errored, or non-OpenAI SSE responses."""
    if response.headers.get_content_type() != "text/event-stream":
        raise ValueError("Expected text/event-stream")
    chunks = 0
    finished = False
    deadline = time.monotonic() + 300
    for line in response:
        if time.monotonic() > deadline:
            raise TimeoutError("Streaming request exceeded 300 seconds")
        line = line.decode("utf-8").strip()
        if not line.startswith("data:"):
            continue
        data = line[5:].strip()
        if data == "[DONE]":
            if not chunks or not finished:
                raise ValueError("Stream completed without content and a finish reason")
            return chunks
        event = json.loads(data)
        if "error" in event:
            raise ValueError("Provider returned an SSE error")
        for choice in event.get("choices", []):
            if choice.get("delta", {}).get("content"):
                chunks += 1
            finished |= choice.get("finish_reason") is not None
    raise ValueError("Stream ended without [DONE]")


def open_response(request):
    # A programmed load balancer can still be propagating backend health.
    # Retry only availability errors before receiving a successful response.
    for attempt in range(6):
        try:
            return urllib.request.urlopen(request, timeout=60)
        except urllib.error.HTTPError as error:
            if error.code not in (502, 503, 504) or attempt == 5:
                raise
            error.close()
        except urllib.error.URLError:
            if attempt == 5:
                raise
        time.sleep(5)
    raise AssertionError("unreachable")


def smoke(endpoint, model, token_parameter="max_tokens", max_output_tokens=128):
    for streaming in (False, True):
        payload = {
            "model": model,
            "messages": [{"role": "user", "content": "Count from one to ten."}],
            token_parameter: max_output_tokens,
            "stream": streaming,
        }
        if model.startswith("gemini-2.5-flash"):
            # Keep this tiny smoke request's token budget available for output.
            payload["thinking_config"] = {"thinkingBudget": 0}
        request = urllib.request.Request(
            endpoint.rstrip("/") + "/v1/chat/completions",
            data=json.dumps(payload).encode(),
            headers={"Content-Type": "application/json"},
        )
        try:
            with open_response(request) as response:
                if streaming:
                    chunks = check_stream(response)
                    print(f"Streaming request passed ({chunks} content chunks)")
                else:
                    result = json.load(response)
                    if "error" in result or not result.get("choices", [{}])[0].get("message", {}).get("content"):
                        raise ValueError("Missing synchronous completion")
                    print("Synchronous request passed")
        except urllib.error.HTTPError as error:
            # Do not print response bodies: they can include provider request details.
            raise RuntimeError(f"Gateway returned HTTP {error.code}") from None


def add_client_arguments(parser):
    """Define request options shared by direct and in-cluster execution."""
    parser.add_argument("--model", help="Model ID (required for real-provider tests)")
    parser.add_argument("--token-parameter", choices=("max_tokens", "max_completion_tokens"),
                        default="max_tokens", help="Request field for the token budget")
    parser.add_argument("--max-output-tokens", type=int, default=128,
                        help="Positive token budget, including reasoning where applicable (default: 128)")


def validate_client_arguments(parser, args):
    if not args.model:
        parser.error("--model is required")
    if args.max_output_tokens <= 0:
        parser.error("--max-output-tokens must be positive")


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, allow_abbrev=False)
    parser.add_argument("--endpoint", required=True, help="Base URL; append /openai or /anthropic for optional routes")
    parser.add_argument("--metrics-endpoint")
    add_client_arguments(parser)
    args = parser.parse_args(argv)
    validate_client_arguments(parser, args)
    return args


def main():
    args = parse_args()
    smoke(args.endpoint, args.model, args.token_parameter, args.max_output_tokens)
    if args.metrics_endpoint:
        with urllib.request.urlopen(args.metrics_endpoint, timeout=10) as response:
            metrics = response.read().decode()
        if "# TYPE" not in metrics or "agentgateway" not in metrics:
            raise ValueError("Missing agentgateway Prometheus metrics")
        print("Prometheus metrics passed")


if __name__ == "__main__":
    main()
