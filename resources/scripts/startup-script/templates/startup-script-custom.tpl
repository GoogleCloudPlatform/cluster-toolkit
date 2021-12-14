

stdlib::run_playbook() {
  if [ ! "$(which ansible-playbook)" ]; then
    stdlib::error "ansible-playbook not found"\
    "Please install ansible before running ansible-local runners."
    exit 1
  fi
  /usr/bin/ansible-playbook --connection=local --inventory=localhost, --limit localhost $1
}

stdlib::runner() {

  type=$1
  object=$2
  destination=$3
  tmpdir=$4

  destpath="$(dirname $destination)"
  filename="$(basename $destination)"

  if [ "$destpath" = "." ]; then
    destpath=$tmpdir
  fi

  stdlib::get_from_bucket -u "gs://${bucket}/$object" -d "$destpath" -f "$filename"

  case "$1" in
    ansible-local) stdlib::run_playbook "$destpath/$filename";;
    # shellcheck source=/dev/null
    shell)  source "$destpath/$filename";;
  esac
}

stdlib::load_runners(){
  tmpdir="$(mktemp -d)"

  stdlib::debug "=== BEGIN Running runners ==="

  %{for p in runners ~}
  stdlib::runner ${p.type} ${p.object} ${p.destination} $${tmpdir}
  %{endfor ~}

  stdlib::debug "=== END Running runners ==="
}

