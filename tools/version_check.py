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

import re


def parse(s):
    match = re.match(r"\s*v?(?P<release>\d+(?:\.\d+)*)\s*", s, re.VERBOSE | re.IGNORECASE)
    if not match:
        raise ValueError(f"Invalid version: '{s}'")
    parts = [int(p) for p in match.group('release').split('.')][:3]
    return tuple(parts + [0] * (3 - len(parts)))

def meet(version, requirement):
    return parse(version) >= parse(requirement)

if __name__ == "__main__":
    """
    Tests:
    assert parse("1") == (1, 0, 0)
    assert parse("1.2") == (1, 2, 0)
    assert parse("1.2.3") == (1, 2, 3)
    assert parse("v1.2") == (1, 2, 0)
    assert meet("v1.2", "v1.2.0")
    assert not meet("v1.2", "v1.2.1")
    """

    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument("version", type=str)
    parser.add_argument("requirement", type=str)
    args = parser.parse_args()
    if meet(args.version, args.requirement):
        print("yes")
    else:
        print("no")
