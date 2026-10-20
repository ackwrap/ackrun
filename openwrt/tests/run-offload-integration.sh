#!/bin/sh
# Run only in a disposable OpenWrt guest. No shared UCI or network is writable.
set -eu
PATH=/usr/sbin:/usr/bin:/sbin:/bin
export PATH

if [ "${1:-}" = --inside ]; then
    [ "$(readlink /proc/self/ns/net)" != "$ACKWRAP_OUTER_NET" ]
    [ "$(readlink /proc/self/ns/mnt)" != "$ACKWRAP_OUTER_MNT" ]
    [ "$(readlink /proc/self/ns/net)" != "$(readlink /proc/1/ns/net)" ]
    [ "$(readlink /proc/self/ns/mnt)" != "$(readlink /proc/1/ns/mnt)" ]
    mount --make-rprivate /
    work=$ACKWRAP_OPENWRT_WORK
    mkdir -p "$work/config" "$work/tmp" "$work/evidence"
    # Never copy the guest's firewall or user includes. The Go fixture writes
    # synthetic UCI with explicit veth devices and auto_includes disabled.
    mount --bind "$work/config" /etc/config
    mount --bind "$work/tmp" /tmp
    mkdir -p /tmp/run /tmp/.uci /tmp/lock /var/run/ubus
    # A fresh sysfs makes fw4 see veth devices in this network namespace.
    mount -t sysfs -o nosuid,nodev,noexec sysfs /sys
    ubusd -s /var/run/ubus/ubus.sock &
    ubus_pid=$!
    trap 'kill "$ubus_pid" 2>/dev/null || true' EXIT HUP INT TERM
    # Output is captured by the outer shell without a pipeline masking status.
    export ACKWRAP_NFT_GUARD_TEST=1 ACKWRAP_OPENWRT_INTEGRATION=1
    "$ACKWRAP_OPENWRT_BINARY" -test.v -test.count=1 -test.timeout=25m \
        -test.run='^TestNFTGuardPackets$/^openwrt-real-fw4$'
    exit 0
fi

[ "$(id -u)" = 0 ] || { echo 'Requires root inside a disposable VM' >&2; exit 2; }
[ -f /etc/openwrt_release ] || { echo 'Requires real OpenWrt' >&2; exit 2; }
[ "$(cat /etc/ackrun-disposable-vm 2>/dev/null)" = ackrun-openwrt-integration-v1 ] || {
    echo 'Missing disposable-VM marker; see README.md' >&2; exit 2;
}
# Reject physical hardware even if somebody copied the marker there.
case "$(cat /sys/class/dmi/id/sys_vendor /sys/class/dmi/id/product_name 2>/dev/null || true)" in
    *QEMU*|*KVM*|*Bochs*) ;;
    *) echo 'This runner requires a disposable x86 QEMU/KVM guest' >&2; exit 2 ;;
esac
for command in unshare mount nsenter ip nft python3 ubusd sha256sum readlink sysctl cmp grep; do
    command -v "$command" >/dev/null || { echo "Missing $command" >&2; exit 2; }
done
[ -x /sbin/uci ] && [ -x /sbin/fw4 ] && [ -r /proc/net/nf_conntrack ] || {
    echo 'Requires UCI, firewall4, nft JSON and CONFIG_NF_CONNTRACK_PROCFS' >&2; exit 2;
}
# /var/run must resolve inside our private /tmp; otherwise fw4 could overwrite
# the guest's runtime state even with isolated /etc/config and nftables.
case "$(readlink -f /var/run)" in
    /tmp/*) ;;
    *) echo 'Expected OpenWrt /var/run under /tmp' >&2; exit 2 ;;
esac
script=$(readlink -f "$0")
bundle=$(dirname "$script")
export ACKWRAP_OPENWRT_BINARY="$bundle/service.test"
[ -x "$ACKWRAP_OPENWRT_BINARY" ] || { echo 'Build service.test first' >&2; exit 2; }
case "$bundle" in /tmp|/tmp/*) echo 'Copy the bundle under /root; /tmp will be hidden' >&2; exit 2 ;; esac
export ACKWRAP_OPENWRT_WORK
ACKWRAP_OPENWRT_WORK=$(mktemp -d /root/ackrun-openwrt-it.XXXXXX)
export ACKWRAP_OUTER_NET ACKWRAP_OUTER_MNT
ACKWRAP_OUTER_NET=$(readlink /proc/self/ns/net)
ACKWRAP_OUTER_MNT=$(readlink /proc/self/ns/mnt)
work=$ACKWRAP_OPENWRT_WORK
# The whole ruleset can change counters asynchronously; use stateless output.
sha256sum /etc/config/firewall > "$work/outer-firewall.before"
uci changes firewall > "$work/outer-pending.before"
nft -s list ruleset > "$work/outer-nft.before"
{
    cat /etc/openwrt_release
    uname -a
    nft --version
    uci -V 2>&1 || true
    if command -v opkg >/dev/null; then opkg list-installed; fi
    if command -v apk >/dev/null; then apk info -v; fi
    sha256sum /sbin/uci /sbin/fw4 "$ACKWRAP_OPENWRT_BINARY"
    cat "$bundle/source-commit.txt" 2>/dev/null || true
} > "$work/environment.txt"
rc=0
unshare -m -n sh "$script" --inside > "$work/test.log" 2>&1 || rc=$?
cat "$work/test.log"
# Missing capabilities, skipped tests and an accidentally stale test binary
# cannot produce a green integration result.
for scenario in pending-before pending-during committed-unrelated committed-offload reload-pause reload-restore corrupt-record permanent-policy kill-intent kill-commit kill-reload kill-guard; do
    if ! grep -F -- "--- PASS: TestNFTGuardPackets/openwrt-real-fw4/$scenario " "$work/test.log" >/dev/null; then
        echo "FAIL: missing real integration result for $scenario" >&2
        rc=1
    fi
done
if grep -F -- '--- SKIP:' "$work/test.log" >/dev/null; then rc=1; fi
sha256sum /etc/config/firewall > "$work/outer-firewall.after"
uci changes firewall > "$work/outer-pending.after"
nft -s list ruleset > "$work/outer-nft.after"
for item in firewall pending nft; do
    if ! cmp -s "$work/outer-$item.before" "$work/outer-$item.after"; then
        echo "FAIL: outer $item changed (evidence: $work)" >&2
        rc=1
    fi
done
if [ "$rc" = 0 ]; then
    echo "PASS: real OpenWrt offload integration; evidence: $work"
else
    echo "FAIL (exit $rc): evidence: $work" >&2
fi
exit "$rc"
