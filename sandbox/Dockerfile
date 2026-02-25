FROM docker/sandbox-templates:claude-code

USER root

# Install KVM tools and ensure kvm group exists
RUN apt-get update && apt-get install -y --no-install-recommends \
    qemu-utils \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd -f -g 36 kvm

# Add the agent user to the kvm group
RUN usermod -aG kvm agent

# Entrypoint script that creates /dev/kvm if the kernel supports it
COPY entrypoint-kvm.sh /usr/local/bin/entrypoint-kvm.sh
RUN chmod +x /usr/local/bin/entrypoint-kvm.sh

# Use the KVM entrypoint, then exec the original entrypoint/command
ENTRYPOINT ["/usr/local/bin/entrypoint-kvm.sh"]

USER agent
