#!/usr/bin/env bash
# scripts/local_setup.sh

set -euo pipefail

# record start time
START_TS=$(date +%s)

# $1 = WORKDIR
# $2 = CLEAN_MODE (true/false)
# $3 = DJANGO_SUPERUSER_PASSWORD
# $4 = DJANGO_SUPERUSER_USERNAME
# $5 = DJANGO_SUPERUSER_EMAIL
# $6 = DNS_HOSTNAME (optional)
# $7 = PROJECT_ID
# $8 = DEPLOYMENT_NAME

WORKDIR="$1"
CLEAN_MODE="$2"
DJANGO_SUPERUSER_PASSWORD="${3:-admin}"
DJANGO_SUPERUSER_USERNAME="${4:-admin}"
DJANGO_SUPERUSER_EMAIL="${5:-admin@example.com}"
DNS_HOSTNAME="${6}"
PROJECT_ID="${7}"
DEPLOYMENT_NAME="${8:-local-dev-project}"

PROJECT_ROOT=$(git rev-parse --show-toplevel)
OFE_SRC="${PROJECT_ROOT}/community/front-end/ofe"
TMP_OFE_DIR="${WORKDIR}/community/front-end/ofe"

export GOOGLE_CLIENT_ID="PLACEHOLDER"
export GOOGLE_CLIENT_SECRET="PLACEHOLDER"

echo ""
echo "[local_setup] WORKDIR: ${WORKDIR}"
echo "[local_setup] CLEAN_MODE: ${CLEAN_MODE}"
echo "[local_setup] DJANGO_SUPERUSER_PASSWORD: ${DJANGO_SUPERUSER_PASSWORD}"
echo "[local_setup] DJANGO_SUPERUSER_USERNAME: ${DJANGO_SUPERUSER_USERNAME}"
echo "[local_setup] DJANGO_SUPERUSER_EMAIL: ${DJANGO_SUPERUSER_EMAIL}"
echo "[local_setup] DNS_HOSTNAME: ${DNS_HOSTNAME}"
echo "[local_setup] PROJECT_ROOT: ${PROJECT_ROOT}"
echo "[local_setup] OFE_SRC: ${OFE_SRC}"
echo "[local_setup] TMP_OFE_DIR: ${TMP_OFE_DIR}"
echo "[local_setup] PROJECT_ID: ${PROJECT_ID}"
echo "[local_setup] DEPLOYMENT_NAME: ${DEPLOYMENT_NAME}"
echo ""

mkdir -p "${WORKDIR}"
if [[ "${CLEAN_MODE}" == "true" ]]; then
  echo "[local_setup] CLEAN_MODE is set. Deleting ${WORKDIR}"
  echo ""
  rm -rf "${WORKDIR}"
  mkdir -p "${WORKDIR}"
elif [[ -f "${TMP_OFE_DIR}/website/db.sqlite3" ]]; then
  echo "[local_setup] Warning: existing DB found in ${TMP_OFE_DIR}"
	echo "[local_setup] Warning: Database file exists. Use --clean flag to start fresh."
  echo ""
	read -r -p "Do you want to clean the environment and start fresh? [y/N] " response
	case "$response" in
		[yY][eE][sS]|[yY])
			echo "Cleaning up existing environment..."
      echo ""
			rm -rf "${WORKDIR}"
			mkdir -p "${WORKDIR}"
			CLEAN_MODE=true
			;;
	*)
		read -r -p "Do you want to continue with existing database? [y/N] " continue_response
		case "$continue_response" in
			[yY][eE][sS]|[yY])
				echo "Continuing with existing database..."
				;;
			*)
				echo "Exiting. Run with --clean flag to start fresh."
				exit 1
				;;
		esac
		;;
	esac
fi

# Todo: OAuth test setup for local dev server
if [[ -z "${DNS_HOSTNAME:-}" ]]; then 
  echo "[local_setup] DNS_HOSTNAME not set; skipping OAuth setup."
  export GOOGLE_CLIENT_ID="" GOOGLE_CLIENT_SECRET=""
else
  # Todo: OAuth test setup for local dev server
  echo "[local_setup] OAuth secrets can be added manually for dev env."
fi

echo "[local_setup] GOOGLE_CLIENT_ID: ${GOOGLE_CLIENT_ID}"
echo "[local_setup] GOOGLE_CLIENT_SECRET: ${GOOGLE_CLIENT_SECRET}"  

# Copy the project files to the workdir and create a virtualenv
echo "[local_setup] rsync from ${PROJECT_ROOT} to ${WORKDIR}"
rsync -a --progress \
  --exclude=.terraform \
  --exclude=.terraform.lock.hcl \
  --exclude=tf \
  "${PROJECT_ROOT}/" "${WORKDIR}/"

cd "${TMP_OFE_DIR}"
mkdir -p run

# virtualenv
if [[ ! -d venv ]]; then
  python3 -m venv venv
fi
source venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt

# spack setup
mkdir -p dependencies
cd dependencies
if [[ ! -d spack ]]; then
  git clone -b v0.21.0 --depth 1 https://github.com/spack/spack.git
fi
cd ..

# build gcluster
cd "${WORKDIR}"
if ! command -v go &>/dev/null; then
  echo "[local_setup] ERROR: go not installed" >&2
  exit 1
fi
make gcluster
export PATH="$PATH:$(pwd)/bin"

echo "[local_setup] gcluster: $(which gcluster)"
echo "[local_setup] version: $(gcluster --version)"

# build configuration.yaml file for gcluster
BASE_DIR="${TMP_OFE_DIR}"
echo "[local_setup] Generating configuration file for backend..."
cd "${TMP_OFE_DIR}"
cat > configuration.yaml <<EOL
config:
  server:
    host_type: "local"
    runtime_mode: "local"
    gcp_project: "${PROJECT_ID:-local-dev-project}"
    gcs_bucket: "local-dev-bucket"
    c2_topic: "local-dev-topic"
    deployment_name: "${DEPLOYMENT_NAME:-local-dev}"
    gcluster_path: "$(which gcluster)"
    gcluster_version: |
$(gcluster --version | sed 's/^/      /')
    baseDir: "${BASE_DIR}"

EOL

cd "${TMP_OFE_DIR}/website"

# Check if this is the first run so the user creation only runs once
IS_FIRST_RUN=false
if [[ ! -f db.sqlite3 ]]; then
  IS_FIRST_RUN=true
fi

# Django migrations and setup
python manage.py makemigrations ghpcfe
python manage.py migrate

# superuser
if [[ "${CLEAN_MODE}" == "true" || "${IS_FIRST_RUN}" == "true" ]]; then
  echo "[local_setup] Creating superuser with username: ${DJANGO_SUPERUSER_USERNAME}"
  echo "[local_setup] Creating superuser with email: ${DJANGO_SUPERUSER_EMAIL}"
  export DJANGO_SUPERUSER_PASSWORD="${DJANGO_SUPERUSER_PASSWORD}"
  python manage.py createsuperuser \
    --username "${DJANGO_SUPERUSER_USERNAME}" \
    --email    "${DJANGO_SUPERUSER_EMAIL}" \
    --noinput
fi

# custom setup, collectstatic, seed, runserver
python manage.py custom_setup_command "${GOOGLE_CLIENT_ID}" "${GOOGLE_CLIENT_SECRET}" "${DNS_HOSTNAME}" --traceback
python manage.py collectstatic --noinput
python manage.py seed_workbench_presets

END_TS=$(date +%s)
ELAPSED=$((END_TS - START_TS))

echo ""
echo "[local_setup] Done. Starting dev server now."
echo "[local_setup] Total local deployment time: ${ELAPSED}s"
echo ""

if [[ "${DNS_HOSTNAME}" == "localhost" ]]; then
  echo "[local_setup] Running dev server at http://localhost:8000"
  exec python manage.py runserver
else
  echo "[local_setup] Running dev server at https://${DNS_HOSTNAME}:8000"
  # Todo: add cert and key to run server with OAuth
  # exec python manage.py runserver --cert run/cert.pem --key run/key.pem 0.0.0.0:8000
  exec python manage.py runserver
fi
