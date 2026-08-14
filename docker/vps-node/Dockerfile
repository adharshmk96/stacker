# A throwaway "VPS" for local testing: sshd with password auth on, so
# ssh-copy-id can install a key the same way it would on a real host.
FROM ubuntu:24.04

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-server sudo ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd

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
RUN ssh-keygen -A

EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]
