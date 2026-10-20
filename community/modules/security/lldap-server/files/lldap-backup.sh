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

# Uploads an online backup of the LLDAP database to GCS.
# The archive holds users.db (SQLite online backup), the key seed and JWT
# secret (needed to use the stored password hashes) and the TLS CA and server
# keys. Treat the bucket as sensitive.
set -euo pipefail

BASE=/var/lib/lldap
BUCKET="${LLDAP_BACKUP_BUCKET:?LLDAP_BACKUP_BUCKET is not set}"

WORK=$(mktemp -d "$BASE/backup/run.XXXXXX")
trap 'rm -rf "$WORK"' EXIT

sqlite3 "$BASE/data/users.db" ".backup '$WORK/users.db'"
if [ "$(sqlite3 "$WORK/users.db" 'PRAGMA integrity_check;')" != "ok" ]; then
	echo "lldap-backup: integrity check of the backup copy failed" >&2
	exit 1
fi
cp "$BASE/secrets/key_seed" "$BASE/secrets/jwt_secret" "$WORK/"
cp -r "$BASE/tls" "$WORK/tls"

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
ARCHIVE="$WORK/lldap-$STAMP.tar.gz"
tar -C "$WORK" -czf "$ARCHIVE" users.db key_seed jwt_secret tls
python3 /usr/local/lib/lldap/lldap_gcp.py gcs-upload "$BUCKET" "lldap/lldap-$STAMP.tar.gz" "$ARCHIVE"
echo "lldap-backup: uploaded gs://$BUCKET/lldap/lldap-$STAMP.tar.gz"
