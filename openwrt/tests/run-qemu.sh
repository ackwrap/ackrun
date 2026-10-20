#!/bin/sh
# Uses a prepared OpenWrt x86/64 generic-ext4-combined BIOS image. Snapshot
# writes are discarded; SSH is loopback-only; there are no host shared folders.
set -eu
[ "$#" -ge 1 ] && [ "$#" -le 2 ] || { echo "Usage: $0 IMAGE [SSH_PORT]" >&2; exit 2; }
image=$(readlink -f "$1")
port=${2:-22222}
case "$port" in ''|*[!0-9]*) echo 'Invalid SSH port' >&2; exit 2 ;; esac
[ "$port" -ge 1024 ] && [ "$port" -le 65535 ] || exit 2
[ -f "$image" ] || exit 2
echo "Disposable OpenWrt console; SSH at 127.0.0.1:$port (guest 192.168.1.1)."
exec qemu-system-x86_64 -machine q35 -accel tcg -m 2048 -smp 2 -nographic \
    -drive "file=$image,format=raw,if=virtio,snapshot=on" \
    -netdev "user,id=mgmt,net=192.168.1.0/24,host=192.168.1.2,dhcpstart=192.168.1.100,hostfwd=tcp:127.0.0.1:$port-192.168.1.1:22" \
    -device virtio-net-pci,netdev=mgmt
