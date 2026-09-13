# Ackwrap OpenWrt packages

`python build.py --target openwrt --arch amd64` creates one package:

- `ackwrap_<version>_<arch>.ipk`

The package installs the embedded-UI binary, UCI defaults, procd service, LuCI Services page, and iStoreOS metadata and icon.

When sing-box is stopped, `/etc/init.d/ackwrap network_repair` safely restores Ackwrap-owned DNS, route, and firewall state without starting the backend or core.

The LuCI **Network Repair / Repair Now** button runs `/etc/init.d/ackwrap network_repair --force`. It stops Ackwrap (including procd respawn) and sing-box, then clears the dedicated sing-box nftables table, current/stale fw4 include, reserved policy-rule priorities 9000–9010 and 32768, table 2022, and identifiable redirect tables. Missing, pending, damaged or mismatched network ownership records do not block this mode. Unrelated route tables, main/local routes and other nftables tables remain in place. Repairs continue through independent cleanup steps and report command errors; incomplete repairs retain recovery state for retry. Services stay stopped after repair and can be started again from LuCI.

The standalone equivalent is `ackwrap network-repair --force` on Linux; stop the Ackwrap service first to prevent a concurrent restart. The command terminates remaining sing-box processes before cleanup. Normal startup and non-force repair retain their ownership checks.
