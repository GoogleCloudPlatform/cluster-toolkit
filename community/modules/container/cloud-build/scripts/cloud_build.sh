#!/bin/bash
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

set -e

# Authenticate gcloud using the active Terraform Google Provider access token / ADC
if [ -n "$ACCESS_TOKEN" ]; then
	export CLOUDSDK_AUTH_ACCESS_TOKEN="$ACCESS_TOKEN"
elif command -v gcloud >/dev/null 2>&1; then
	ADC_TOKEN=$(gcloud auth application-default print-access-token 2>/dev/null || true)
	if [ -n "$ADC_TOKEN" ]; then
		export CLOUDSDK_AUTH_ACCESS_TOKEN="$ADC_TOKEN"
	fi
fi

# Check if images already exist in Artifact Registry
if [ -n "$SKIP_IF_EXISTS" ]; then
	ALL_EXIST=true
	CHECKED_COUNT=0
	IFS=',' read -ra IMGS <<<"$SKIP_IF_EXISTS"
	for IMG in "${IMGS[@]}"; do
		IMG=$(echo "$IMG" | xargs)
		if [ -n "$IMG" ]; then
			CHECKED_COUNT=$((CHECKED_COUNT + 1))
			echo "--> [INFO] Checking if image '$IMG' already exists in Artifact Registry..."
			if ! gcloud artifacts docker images describe "$IMG" --project="$PROJECT_ID" >/dev/null 2>&1; then
				echo "--> [INFO] Image '$IMG' not found."
				ALL_EXIST=false
				break
			fi
		fi
	done
	if [ "$ALL_EXIST" = "true" ] && [ "$CHECKED_COUNT" -gt 0 ]; then
		echo "--> [INFO] All images in skip_if_exists already exist in Artifact Registry. Skipping Cloud Build."
		exit 0
	fi
	echo "--> [INFO] One or more images do not exist. Proceeding with build."
fi

TMP_WORKSPACE=$(mktemp -d)
trap 'rm -rf "$TMP_WORKSPACE"' EXIT

# Assemble gcloud builds submit arguments
BUILD_ARGS=()
BUILD_ARGS+=("--project=$PROJECT_ID")
BUILD_ARGS+=("--region=$REGION")

if [ -n "$CONFIG_CONTENT" ]; then
	CONFIG_FILE="$TMP_WORKSPACE/cloudbuild.yaml"
	printf '%s\n' "$CONFIG_CONTENT" >"$CONFIG_FILE"
	BUILD_ARGS+=("--config=$CONFIG_FILE")
fi

if [ -n "$GCS_STAGING_DIR" ]; then
	BUILD_ARGS+=("--gcs-source-staging-dir=$GCS_STAGING_DIR")
fi

if [ -n "$SERVICE_ACCOUNT" ]; then
	BUILD_ARGS+=("--service-account=$SERVICE_ACCOUNT")
fi

if [ -n "$SUBSTITUTIONS" ]; then
	BUILD_ARGS+=("--substitutions=$SUBSTITUTIONS")
fi

STAGE_DIR="$TMP_WORKSPACE/source_stage"
mkdir -p "$STAGE_DIR"
HAS_LOCAL_SOURCE=0

if [ -n "$REPO_URL" ]; then
	# When REPO_URL is set, cloud_build_dir refers to the path in the remote Git repository.
	# Core source code is cloned remotely. Stage local target_dir (when is_target_dir_local = true) so it can be overlaid in the build.
	if [ -n "$TARGET_DIR" ]; then
		if [ ! -d "$TARGET_DIR" ]; then
			echo "ERROR: Local target directory not found: $TARGET_DIR" >&2
			exit 1
		fi
		echo "--> [INFO] Found local target directory: $TARGET_DIR. Staging for upload..."
		SRC_NAME=$(basename "$TARGET_DIR")
		mkdir -p "$STAGE_DIR/$SRC_NAME"
		cp -r "$TARGET_DIR/." "$STAGE_DIR/$SRC_NAME/"
		HAS_LOCAL_SOURCE=1
	fi

	if [ "$HAS_LOCAL_SOURCE" -eq 1 ]; then
		echo "--> [INFO] Submitting Cloud Build job with local target directory overlay and remote Git repository ($REPO_URL)..."
		gcloud builds submit "$STAGE_DIR" "${BUILD_ARGS[@]}"
	else
		echo "--> [INFO] Submitting Cloud Build job with --no-source (remote Git: $REPO_URL, ref: $REPO_REF)..."
		gcloud builds submit --no-source "${BUILD_ARGS[@]}"
	fi
elif [ -z "$CLOUD_BUILD_DIR" ]; then
	echo "--> [INFO] Submitting Cloud Build job with --no-source (self-contained config)..."
	gcloud builds submit --no-source "${BUILD_ARGS[@]}"
else
	# When REPO_URL is not set, CLOUD_BUILD_DIR is the local source directory.
	if [ ! -d "$CLOUD_BUILD_DIR" ]; then
		echo "ERROR: Local Cloud Build source directory not found: $CLOUD_BUILD_DIR" >&2
		exit 1
	fi
	if [ -n "$TARGET_DIR" ]; then
		if [ ! -d "$TARGET_DIR" ]; then
			echo "ERROR: Local target directory not found: $TARGET_DIR" >&2
			exit 1
		fi
		cp -r "$CLOUD_BUILD_DIR/." "$STAGE_DIR/"
		SRC_NAME=$(basename "$TARGET_DIR")
		mkdir -p "$STAGE_DIR/$SRC_NAME"
		cp -r "$TARGET_DIR/." "$STAGE_DIR/$SRC_NAME/"
		echo "--> [INFO] Submitting Cloud Build job with local source ($CLOUD_BUILD_DIR) and target overlay ($TARGET_DIR)..."
		gcloud builds submit "$STAGE_DIR" "${BUILD_ARGS[@]}"
	else
		echo "--> [INFO] Submitting Cloud Build job with local source: $CLOUD_BUILD_DIR (project: $PROJECT_ID, region: $REGION)..."
		gcloud builds submit "$CLOUD_BUILD_DIR" "${BUILD_ARGS[@]}"
	fi
fi

echo "--> [INFO] Cloud Build completed successfully."
