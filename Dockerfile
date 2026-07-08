FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive \
    container=docker

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        age \
        ca-certificates \
        git \
        openssh-server \
        procps \
        rsync \
        sudo \
        systemd \
        systemd-sysv && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

RUN useradd --create-home --shell /bin/bash skuld-deploy && \
    useradd --create-home --shell /bin/bash skuld-daemon && \
    install -d -m 0700 -o skuld-deploy -g skuld-deploy /home/skuld-deploy/.ssh && \
    install -d -m 0700 -o skuld-daemon -g skuld-daemon /home/skuld-daemon/.config/skuld-cli && \
    install -d -m 0755 /var/run/sshd && \
    printf 'skuld-deploy ALL=(ALL) NOPASSWD: ALL\n' >/etc/sudoers.d/skuld-deploy && \
    chmod 0440 /etc/sudoers.d/skuld-deploy && \
    ln -sf /lib/systemd/system/ssh.service /etc/systemd/system/multi-user.target.wants/ssh.service

VOLUME ["/sys/fs/cgroup"]

STOPSIGNAL SIGRTMIN+3

CMD ["/sbin/init"]
