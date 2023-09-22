#!/bin/sh

FINISH_LINE="startup-script exit status"
ACTIVE=^activ
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

Please see GCP Cloud Console "Logs Explorer" tool or journalctl to determine
the cause of the startup script failure.

The following command can also help display relevant information.

Local shell:
$ sudo journalctl -u google-startup-scripts.service
EOF
)

# Check if end of startup scripts has been detected
IS_ACTIVE=$(systemctl is-active google-startup-scripts.service | grep "${ACTIVE}")
END_FOUND=$(journalctl -b 0 -u google-startup-scripts.service | grep "${FINISH_LINE}")
STATUS=$(echo "${END_FOUND}" | sed -r 's/.*([0-9]+)\s*$/\1/' | uniq)

# Present user with warning if scripts have not completed
if [ -n "${IS_ACTIVE}" ]; then
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
else
	# If there was an error, let the user know
	if [ "${STATUS}" -ne 0 ]; then
		echo
		echo "${ERROR_NOTICE}"
		echo
	fi
fi
