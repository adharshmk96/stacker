# A throwaway "VPS" for local testing. Boots systemd as PID 1 so the box
# behaves like a real host: `systemctl` works, and anything stacker installs
# over SSH (docker, in particular) starts through its own unit file instead
# of needing to be babysat by hand.
FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive
ENV container=docker

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        systemd systemd-sysv dbus dbus-user-session \
        openssh-server sudo ca-certificates curl iproute2 iptables \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd

# Units that are meaningless in a container and only slow the boot down.
RUN systemctl mask \
        systemd-udevd.service systemd-udevd-kernel.socket \
        systemd-udevd-control.socket systemd-modules-load.service \
        sys-kernel-debug.mount sys-kernel-tracing.mount \
        systemd-journald-audit.socket

# stacker/stacker is the login the UI should use; root is left password-locked.
RUN useradd -m -s /bin/bash stacker \
    && echo 'stacker:stacker' | chpasswd \
    && usermod -aG sudo stacker \
    && echo 'stacker ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/stacker

RUN printf '%s\n' \
    'PermitRootLogin no' \
    'PasswordAuthentication yes' \
    'PubkeyAuthentication yes' \
    'UsePAM yes' \
    > /etc/ssh/sshd_config.d/stacker.conf

# Host keys are baked into the image so recreating a container doesn't
# invalidate the known_hosts entry for its port.
RUN ssh-keygen -A && systemctl enable ssh

EXPOSE 22
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
