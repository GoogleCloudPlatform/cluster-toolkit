#!/bin/sh

RESULT_FILE=/usr/local/ghpc/script-complete
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

# Check if end of startup scripts has been detected
if [ -f "${RESULT_FILE}" ]; then
	STATUS=$(cat "${RESULT_FILE}")
	if [ "${STATUS}" != 0 ]; then
		echo
		echo "${ERROR_NOTICE}"
		echo
	fi
fi

# Present user with warning if scripts have not completed
if [ -z "${STATUS}" ]; then
	clear
	echo "${WARNING}"
	echo
	echo "Press any key to continue"
	old=$(stty -g)
	stty raw -echo
	dd bs=1 count=1 2>/dev/null
	stty "$old"
	clear
	run-parts /etc/update-motd.d/
fi
