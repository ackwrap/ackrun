# Ackwrap OpenWrt packages

`python build.py --target openwrt --arch amd64` creates one package:

- `ackwrap_<version>_<arch>.ipk`

The package installs the embedded-UI binary, UCI defaults, procd service, LuCI Services page, and iStoreOS metadata and icon.

When sing-box is stopped, `/etc/init.d/ackwrap network_repair` safely restores Ackwrap-owned DNS, route, and firewall state without starting the backend or core.
