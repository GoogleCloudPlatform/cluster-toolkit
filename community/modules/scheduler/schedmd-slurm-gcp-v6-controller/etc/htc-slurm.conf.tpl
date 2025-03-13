# slurm.conf
# https://slurm.schedmd.com/high_throughput.html

ProctrackType=proctrack/cgroup
SlurmctldPidFile=/var/run/slurm/slurmctld.pid
SlurmdPidFile=/var/run/slurm/slurmd.pid
TaskPlugin=task/affinity,task/cgroup
MaxArraySize=10001
MaxJobCount=500000
MaxNodeCount=65536
MinJobAge=60

#
#
# SCHEDULING
SchedulerType=sched/backfill
SelectType=select/cons_tres
SelectTypeParameters=CR_Core_Memory

#
#
# LOGGING AND ACCOUNTING
SlurmctldDebug=error
SlurmdDebug=error

#
#
# TIMERS
MessageTimeout=60

################################################################################
#              vvvvv  WARNING: DO NOT MODIFY SECTION BELOW  vvvvv              #
################################################################################

SlurmctldHost={control_host}({control_addr})

AuthType=auth/slurm
AuthInfo=cred_expire=120
AuthAltTypes=auth/jwt
CredType=cred/slurm
MpiDefault={mpi_default}
ReturnToService=2
SlurmctldPort={control_host_port}
SlurmdPort=6818
SlurmdSpoolDir=/var/spool/slurmd
SlurmUser=slurm
StateSaveLocation={state_save}

#
#
# LOGGING AND ACCOUNTING
AccountingStorageType=accounting_storage/slurmdbd
AccountingStorageHost={control_host}
ClusterName={name}
SlurmctldLogFile={slurmlog}/slurmctld.log
SlurmdLogFile={slurmlog}/slurmd-%n.log

#
#
# GENERATED CLOUD CONFIGURATIONS
include cloud.conf

################################################################################
#              ^^^^^  WARNING: DO NOT MODIFY SECTION ABOVE  ^^^^^              #
################################################################################
