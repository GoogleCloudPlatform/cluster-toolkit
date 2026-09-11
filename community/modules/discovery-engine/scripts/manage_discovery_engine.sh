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

ACTION="${ACTION:-create}"
TARGET="${TARGET:-engine}"

if [ -z "$PROJECT_ID" ] || [ -z "$LOCATION" ] || [ -z "$COLLECTION" ] || [ -z "$BASE_URL" ] || [ -z "$ENGINE_ID" ]; then
	echo "ERROR: Missing required environment variables (PROJECT_ID, LOCATION, COLLECTION, BASE_URL, ENGINE_ID)" >&2
	exit 1
fi

TOKEN="${ACCESS_TOKEN:-${GOOGLE_OAUTH_ACCESS_TOKEN:-}}"
if [ -z "$TOKEN" ]; then
	TOKEN=$(gcloud auth application-default print-access-token 2>/dev/null || gcloud auth print-access-token 2>/dev/null || true)
fi

if [ -z "$TOKEN" ]; then
	echo "ERROR: Failed to obtain GCP access token using 'gcloud auth application-default print-access-token' or 'gcloud auth print-access-token'." >&2
	echo "Please ensure Application Default Credentials (ADC) or gcloud is authenticated." >&2
	exit 1
fi

ENGINES_URL="https://${BASE_URL}/v1/projects/${PROJECT_ID}/locations/${LOCATION}/collections/${COLLECTION}/engines"
ENGINE_URL="${ENGINES_URL}/${ENGINE_ID}"

if [ "$TARGET" = "engine" ]; then
	if [ "$ACTION" = "create" ]; then
		echo "Checking if Discovery Engine '${ENGINE_ID}' already exists..."
		HTTP_STATUS=$(curl -s --max-time 60 -o /dev/null -w "%{http_code}" \
			-H "Authorization: Bearer ${TOKEN}" \
			-H "x-goog-user-project: ${PROJECT_ID}" \
			"${ENGINE_URL}")

		if [ "$HTTP_STATUS" = "200" ]; then
			echo "Discovery Engine '${ENGINE_ID}' already exists."
			exit 0
		fi

		echo "Creating Discovery Engine '${ENGINE_ID}'..."
		RESPONSE=$(curl -s --max-time 60 -w "\n%{http_code}" -X POST \
			-H "Content-Type: application/json" \
			-H "Authorization: Bearer ${TOKEN}" \
			-H "x-goog-user-project: ${PROJECT_ID}" \
			-d "{
				\"display_name\": \"${ENGINE_ID}\",
				\"data_store_ids\": [],
				\"solution_type\": \"SOLUTION_TYPE_GENERATIVE_CHAT\"
			}" \
			"${ENGINES_URL}?engineId=${ENGINE_ID}")

		HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
		BODY=$(echo "$RESPONSE" | sed '$d')

		if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 300 ]; then
			echo "Discovery Engine '${ENGINE_ID}' created successfully."
		elif [ "$HTTP_CODE" = "409" ]; then
			echo "Discovery Engine '${ENGINE_ID}' already exists."
		else
			echo "ERROR: Failed to create Discovery Engine (HTTP ${HTTP_CODE}): ${BODY}" >&2
			exit 1
		fi

	elif [ "$ACTION" = "destroy" ]; then
		echo "Deleting Discovery Engine '${ENGINE_ID}'..."
		HTTP_CODE=$(curl -s --max-time 60 -o /dev/null -w "%{http_code}" -X DELETE \
			-H "Authorization: Bearer ${TOKEN}" \
			-H "x-goog-user-project: ${PROJECT_ID}" \
			"${ENGINE_URL}")

		if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ] || [ "$HTTP_CODE" = "404" ]; then
			echo "Discovery Engine '${ENGINE_ID}' deleted successfully (or already deleted)."
		else
			echo "WARNING: Failed to delete Discovery Engine '${ENGINE_ID}' (HTTP ${HTTP_CODE})" >&2
		fi
	else
		echo "ERROR: Unknown ACTION '${ACTION}' for engine" >&2
		exit 1
	fi

elif [ "$TARGET" = "assistant" ]; then
	if [ -z "$ASSISTANT_ID" ]; then
		echo "ERROR: ASSISTANT_ID is required for assistant target" >&2
		exit 1
	fi

	ASSISTANTS_URL="${ENGINE_URL}/assistants"
	ASSISTANT_URL="${ASSISTANTS_URL}/${ASSISTANT_ID}"

	if [ "$ACTION" = "create" ]; then
		echo "Checking if Assistant '${ASSISTANT_ID}' already exists..."
		HTTP_STATUS=$(curl -s --max-time 60 -o /dev/null -w "%{http_code}" \
			-H "Authorization: Bearer ${TOKEN}" \
			-H "x-goog-user-project: ${PROJECT_ID}" \
			"${ASSISTANT_URL}")

		if [ "$HTTP_STATUS" = "200" ]; then
			echo "Assistant '${ASSISTANT_ID}' already exists."
		else
			echo "Creating Assistant '${ASSISTANT_ID}'..."
			RESPONSE=$(curl -s --max-time 60 -w "\n%{http_code}" -X POST \
				-H "Content-Type: application/json" \
				-H "Authorization: Bearer ${TOKEN}" \
				-H "x-goog-user-project: ${PROJECT_ID}" \
				-d "{
					\"display_name\": \"${ASSISTANT_ID}\",
					\"web_grounding_type\": \"WEB_GROUNDING_TYPE_UNSPECIFIED\"
				}" \
				"${ASSISTANTS_URL}?assistantId=${ASSISTANT_ID}")

			HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
			BODY=$(echo "$RESPONSE" | sed '$d')

			if [ "$HTTP_CODE" -ge 200 ] && [ "$HTTP_CODE" -lt 300 ]; then
				echo "Assistant '${ASSISTANT_ID}' created successfully."
			elif [ "$HTTP_CODE" = "409" ]; then
				echo "Assistant '${ASSISTANT_ID}' already exists."
			else
				echo "ERROR: Failed to create Assistant (HTTP ${HTTP_CODE}): ${BODY}" >&2
				exit 1
			fi
		fi

		echo "Validating Assistant '${ASSISTANT_ID}' via streamAssist..."
		VALIDATED=false
		for i in 1 2 3 4 5; do
			STREAM_STATUS=$(curl -s --max-time 60 -o /dev/null -w "%{http_code}" -X POST \
				-H "Content-Type: application/json" \
				-H "Authorization: Bearer ${TOKEN}" \
				-H "x-goog-user-project: ${PROJECT_ID}" \
				-d '{"query": {"text": "health check query"}}' \
				"${ASSISTANT_URL}:streamAssist" || true)

			if [ "$STREAM_STATUS" = "200" ]; then
				echo "Assistant '${ASSISTANT_ID}' validated successfully."
				VALIDATED=true
				break
			fi

			if [ "$i" -lt 5 ]; then
				echo "Validation attempt ${i}/5 returned HTTP ${STREAM_STATUS}. Retrying in 2 seconds..."
				sleep 2
			fi
		done

		if [ "$VALIDATED" != "true" ]; then
			echo "ERROR: Discovery Engine API validation failed via streamAssist (HTTP ${STREAM_STATUS}). Please check permissions." >&2
			exit 1
		fi

	elif [ "$ACTION" = "destroy" ]; then
		echo "Deleting Assistant '${ASSISTANT_ID}'..."
		HTTP_CODE=$(curl -s --max-time 60 -o /dev/null -w "%{http_code}" -X DELETE \
			-H "Authorization: Bearer ${TOKEN}" \
			-H "x-goog-user-project: ${PROJECT_ID}" \
			"${ASSISTANT_URL}")

		if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "204" ] || [ "$HTTP_CODE" = "404" ]; then
			echo "Assistant '${ASSISTANT_ID}' deleted successfully (or already deleted)."
		else
			echo "WARNING: Failed to delete Assistant '${ASSISTANT_ID}' (HTTP ${HTTP_CODE})" >&2
		fi
	else
		echo "ERROR: Unknown ACTION '${ACTION}' for assistant" >&2
		exit 1
	fi
else
	echo "ERROR: Unknown TARGET '${TARGET}' (must be 'engine' or 'assistant')" >&2
	exit 1
fi
