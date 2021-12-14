

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
  args=$5

  destpath="$(dirname $destination)"
  filename="$(basename $destination)"

  if [ "$destpath" = "." ]; then
    destpath=$tmpdir
  fi

  stdlib::get_from_bucket -u "gs://${bucket}/$object" -d "$destpath" -f "$filename"

  case "$1" in
    ansible-local) stdlib::run_playbook "$destpath/$filename";;
    # shellcheck source=/dev/null
    shell)  sh -c "source '$destpath/$filename' $args";;
  esac
}

stdlib::load_runners(){
  tmpdir="$(mktemp -d)"

  stdlib::debug "=== BEGIN Running runners ==="

  %{for r in runners ~}
  stdlib::runner "${r.type}" "${r.object}" "${r.destination}" $${tmpdir} "${r.args}"
  %{endfor ~}

  stdlib::debug "=== END Running runners ==="
}

