#!/bin/bash
set -e

# Try to create /dev/kvm if it doesn't exist but the kernel supports it
if [ ! -e /dev/kvm ]; then
    if [ -d /sys/module/kvm ]; then
        echo "[sandbox-kvm] KVM kernel module detected, creating /dev/kvm..."
        mknod /dev/kvm c 10 232 2>/dev/null && chmod 660 /dev/kvm && chgrp kvm /dev/kvm \
            && echo "[sandbox-kvm] /dev/kvm created successfully" \
            || echo "[sandbox-kvm] WARNING: Failed to create /dev/kvm (container may lack CAP_MKNOD)"
    else
        echo "[sandbox-kvm] WARNING: KVM kernel module not loaded, /dev/kvm unavailable"
    fi
else
    echo "[sandbox-kvm] /dev/kvm already exists"
fi

# Report status
if [ -e /dev/kvm ]; then
    echo "[sandbox-kvm] KVM is available: $(ls -la /dev/kvm)"
fi

# Execute the original command (or default shell)
exec "$@"
