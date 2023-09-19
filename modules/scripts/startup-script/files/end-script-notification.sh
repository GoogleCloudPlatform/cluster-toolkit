#!/bin/sh

FINISH_LINE="startup-script exit status"
RESULT_FILE=/usr/local/ghpc/script-complete
NOTICE=$(
	cat <<EOF
** NOTICE **: The VM startup scripts have finished running successfully.  
It is now safe to start using the system.
EOF
)
ERROR_NOTICE=$(
	cat <<EOF
** ERROR **: The VM startup scripts have finished running, but produced an error. 

Please see GCP instance console output or journalctl to determine the cause of
the startup script failure.

The following commands can also help display relevant information (replace the
variables in the <> brackets).

Local shell:
$ sudo journalctl -u google-startup-scripts.service

Cloud Shell (or system with gcloud installed):
$ gcloud compute instances get-serial-port-output <instance name> --port 1 \
         --zone <zone> --project <project id> | grep google_metadata | less
EOF
)
WARNING=$(
	cat <<EOF
** WARNING **: The VM startup scripts for this have not been completed and
system services may not be configured yet.

Attempting to make changes to the system may lead to undefined behavior.
It is advised that you wait for the startup scripts to complete.
An alert will be written to all logged in users confirming that the startup
scripts have completed.

Another way to check the status of the startup scripts is to run:
$ systemctl status google-startup-scripts.service

If the "Active" status is no longer "active", the scripts have completed.
EOF
)

# Check if startup script service is running
SERVICE_STATE=$(systemctl status google-startup-scripts.service | grep "Active: active")
if [ -n "${SERVICE_STATE}" ]; then
	# We can delete the result file since we will create a new one later on
	if [ -f "${RESULT_FILE}" ]; then
		rm -rf "${RESULT_FILE}"
	fi
else
	# If not running, exit
	# We assume this should not be run separate of the startup script service.
	exit 0
fi

# Print message for users that logged in before startup scripts started
NUM_LOGGED_IN=$(($(w | wc -l) - 2))
if [ "${NUM_LOGGED_IN}" -gt 0 ]; then
	wall -n "${WARNING}"
fi

# Loop until end of service is found
while :; do
	# Check if end of startup scripts has been detected
	ser_log=$(journalctl -b 0 -u google-startup-scripts.service | grep "${FINISH_LINE}")
	STATUS=$(echo "${ser_log}" | sed -r 's/.*([0-9]+)\s*$/\1/' | uniq)

	# If the scripts are running, present user with a notice when the scripts have
	# completed
	if [ -n "${STATUS}" ]; then
		if [ "${STATUS}" -eq 0 ]; then
			wall -n "${NOTICE}"
		else
			wall -n "${ERROR_NOTICE}"
		fi
		echo "${STATUS}" >>"${RESULT_FILE}"
		exit 0
	fi
	sleep 5
done
