#! /bin/bash

stdlib::run_playbook() {
  if [ ! "$(which ansible-playbook)" ]; then
    stdlib::error "ansible-playbook not found"\
    "Please install ansible before running ansible-local runners."
    exit 1
  fi
  /usr/bin/ansible-playbook --connection=local --inventory=localhost, --limit localhost $1
}

stdlib::runner() {
  stdlib::get_from_bucket -u "gs://${bucket}/$2" -d "$3"

  case "$1" in
    ansible-local) stdlib::run_playbook "$3/$2";;
    # shellcheck source=/dev/null
    shell)  source "$3/$2";;
  esac
}

stdlib::init_gsutil_crcmod_el

tmpdir="$(mktemp -d)"

stdlib::debug "=== BEGIN Running runners ==="

%{for p in runners ~}
stdlib::runner ${p.type} ${p.object} $${tmpdir}
%{endfor ~}

stdlib::debug "=== END Running runners ==="
echo "Finished with startup-script-custom"
