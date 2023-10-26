FROM rockylinux/rockylinux:9

RUN \
    dnf install -y epel-release && dnf install -y apptainer-suid

RUN \
    apptainer config fakeroot --add root

ENTRYPOINT ["apptainer"]
