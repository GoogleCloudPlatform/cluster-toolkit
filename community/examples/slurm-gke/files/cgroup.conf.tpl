# cgroup.conf
# https://slurm.schedmd.com/cgroup.conf.html

CgroupPlugin=autodetect
IgnoreSystemd=yes
# EnableControllers=yes
ConstrainCores=yes
ConstrainRamSpace=yes
ConstrainSwapSpace=no
ConstrainDevices=yes
