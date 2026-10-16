# Ackwrap OpenWrt packages

`python build.py --target openwrt --arch amd64` creates one package:

- `ackwrap_<version>_<arch>.ipk`

The package installs the embedded-UI binary, UCI defaults, procd service, LuCI Services page, and iStoreOS metadata and icon.

The General settings page includes optional device traffic leak protection. It
requires OpenWrt/firewall4, rule-mode TUN routing, enabled device bindings and
disabled flow offload. Existing installations default to off. Stop the core
before toggling protection or changing the protected device address set.
The independent `ackwrap_device_protection` nftables table blocks protected
IPv4/IPv6 sources from forwarding outside `tun0` and keeps their DNS redirected
to the device resolver even when the daemon/core stops. Router-local management
remains reachable; independent local proxy services are outside this policy.
Only registered device addresses are covered, including every IPv6 address in
use. Device addresses should be stable.

Persistent policy is stored at `/etc/ackwrap-protection/guard.nft`, independently
of the configurable data directory. The package's fw4 `ruleset-pre` include loads
it on boot/reload without the daemon. Enabling verifies both the live nftables
table and the fw4 include. Removing firewall rules manually or stopping/flushing
the system firewall is outside this protection; the UI reports missing rules.
Routine and forced network repair preserve device protection; explicitly disable
the switch or use **Force Reset** to release it. Updating/disabling a device rule while the core is
stopped updates its protected address set; removing the last device requires
disabling protection first.

When sing-box is stopped, `/etc/init.d/ackwrap network_repair` safely restores Ackwrap-owned DNS, route, and firewall state without starting the backend or core.

The LuCI **Network Repair / Force Repair** button runs `/etc/init.d/ackwrap network_repair --force`. It stops Ackwrap (including procd respawn) and sing-box, then clears the dedicated sing-box nftables table, current/stale fw4 include, reserved policy-rule priorities 9000–9010 and 32768, table 2022, and identifiable redirect tables. Missing, pending, damaged or mismatched network ownership records do not block this mode. Unrelated route tables, main/local routes and other nftables tables remain in place. Repairs continue through independent cleanup steps and report command errors; incomplete repairs retain recovery state for retry. Services stay stopped after repair and can be started again from LuCI.

The adjacent **Force Reset** button runs `/etc/init.d/ackwrap network_repair --force-reset`.
It performs the same forced repair and additionally disables the saved device leak
protection setting, removes its persistent policy (even if damaged), and releases
Ackwrap-owned device protection and DNS redirect tables. Subscriptions, nodes,
device bindings and other settings are retained. Services stay stopped. If the
protection setting cannot be saved, the guard is retained and failure is reported;
retry after fixing the database or storage error. A successful reset remains off
when the daemon starts again, until the user explicitly enables protection.

The standalone equivalents are `ackwrap network-repair --force` and `ackwrap network-repair --force-reset` on Linux; stop the Ackwrap service first to prevent a concurrent restart. The commands terminate remaining sing-box processes before cleanup. Normal startup and non-force repair retain their ownership checks.

LuCI **Save & Apply** commits the settings and explicitly runs `apply_settings`, even when the enabled checkbox has not changed. This starts an enabled service or stops a disabled service and reports the observed process state. **Save** only stages settings. Repair and service-status checks identify `/usr/bin/ackwrap` by its executable path so BusyBox does not mistake the identically named init script for the daemon or kill the repair caller before it returns its result.
