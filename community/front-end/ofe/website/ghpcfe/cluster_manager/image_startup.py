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

"""Startup script for the Packer builder VM of a custom image.

Packer's googlecompute builder waits for its startup-script wrapper to set the
instance metadata key `startup-script-status` to `done` or `error`. In
packer-plugin-googlecompute v1.2.7 the wrapper names the instance with bash's
$HOSTNAME, which on some boots of the Slurm base images is still the FQDN; the
API rejects that name, the status is never set, and the build waits forever.

wrap_startup_script() runs the image's startup script inside an outer script
that reports its result, success or failure, under the instance name from the
metadata server.

Temporary: the upstream fix (hashicorp/packer-plugin-googlecompute#355) is
merged but not released. The `~> 1.2.5` constraint in
modules/packer/custom-image/versions.pkr.hcl picks up a fixed release, after
which this wrapper can be removed.

Standard library only, so tests can load it without Django.
"""

_DELIMITER = "OFE_IMAGE_STARTUP_SCRIPT_EOF"

_TEMPLATE = """#!/bin/bash
# Runs the image's startup script, then sets startup-script-status for Packer
# under the instance's real name (see ghpcfe/cluster_manager/image_startup.py).
md() {{
	curl -sSf -H "Metadata-Flavor: Google" \\
		"http://metadata.google.internal/computeMetadata/v1/instance/$1"
}}

script=$(mktemp) || exit 1
cat >"$script" <<'{delimiter}'
{script}
{delimiter}
chmod +x "$script"
"$script"
rc=$?
rm -f "$script"

if [ "$rc" -eq 0 ]; then status="done"; else status="error"; fi
if name=$(md name) && zone=$(md zone); then
	gcloud compute instances add-metadata "$name" --zone "${{zone##*/}}" \\
		--metadata "startup-script-status=$status" ||
		echo "OFE: could not set startup-script-status=$status" >&2
else
	echo "OFE: could not read instance name or zone from metadata" >&2
fi
exit "$rc"
"""


def wrap_startup_script(script):
    """Return `script` wrapped so that its result reaches Packer."""
    if _DELIMITER in script.splitlines():
        raise ValueError(f"startup script contains the line {_DELIMITER}")
    return _TEMPLATE.format(delimiter=_DELIMITER, script=script.rstrip("\n"))
