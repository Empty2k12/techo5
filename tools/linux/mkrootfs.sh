#!/bin/sh
# mkrootfs.sh — build the TECHO5 persistent root filesystem as a tarball.
#
# Runs ON THE DEVICE (or in any armv7 Alpine environment with network): the
# build host has no armv7 chroot, and apk needs one to install packages
# properly, with their triggers and a package database — which is what makes
# `apk add bluez` possible later. tools/linux/deploy-rootfs.sh ships the inputs
# here and runs this.
#
#   mkrootfs.sh -i <indir> -o <out.tar.gz> [-w <workdir>] [-V <version>] [-z <timezone>]
#
# <indir> layout (what deploy-rootfs.sh stages):
#   bin/techo5 bin/fbprobe bin/audioprobe bin/rebootto      Go binaries, armv7
#   tools/slotctl tools/techo5-lib.sh tools/packages-rootfs.txt
#   overlay/                                               tools/linux/rootfs from the repo
#   inputs/alpine-minirootfs-*-armv7.tar.gz
#   inputs/vendor.tar.gz                                   LineageOS system: vendor/ (modules, firmware, audio tuning)
#   inputs/apks312/*.apk                                   wpa_supplicant 2.9 + libssl1.1 + libcrypto1.1
#   inputs/authorized_keys                                 SSH public key(s) for root
set -e

IN=; OUT=; WORK=/data/techo5-linux/build; VERSION=dev; TZNAME=UTC
while [ $# -gt 0 ]; do
	case "$1" in
	-i) IN=$2; shift 2;;
	-o) OUT=$2; shift 2;;
	-w) WORK=$2; shift 2;;
	-V) VERSION=$2; shift 2;;
	-z) TZNAME=$2; shift 2;;
	*) echo "mkrootfs: unknown argument $1" >&2; exit 1;;
	esac
done
[ -n "$IN" ] && [ -n "$OUT" ] || { echo "usage: mkrootfs.sh -i indir -o out.tar.gz [-w workdir] [-V version] [-z timezone]" >&2; exit 1; }

say() { echo "mkrootfs: $*"; }
R=$WORK/root
rm -rf "$R"
mkdir -p "$R"

mini=$(ls "$IN"/inputs/alpine-minirootfs-*-armv7.tar.gz | head -1)
say "base: $(basename "$mini")"
tar -xzf "$mini" -C "$R"

# Package installation needs a resolver inside the root.
cp /etc/resolv.conf "$R/etc/resolv.conf"

pkgs=$(sed 's/#.*//' "$IN/tools/packages-rootfs.txt" | tr '\n' ' ')
say "apk add: $pkgs"
apk --root "$R" --no-cache add $pkgs
say "apk add (local): $(ls "$IN"/inputs/apks312/*.apk | xargs -n1 basename | tr '\n' ' ')"
apk --root "$R" --no-cache add --allow-untrusted "$IN"/inputs/apks312/*.apk
apk --root "$R" info -v | sort > "$R/etc/techo5-packages"

# Vendor tree: Wi-Fi/BT modules, firmware (firmware_class.path=/vendor/firmware on
# the kernel command line), the audio tuning the daemon reads.
say "vendor tree"
tar -xzf "$IN/inputs/vendor.tar.gz" -C "$R" vendor
[ -e "$R/vendor/lib/modules/mt76x8_wlan.ko" ] || { echo "mkrootfs: vendor tree has no mt76x8_wlan.ko" >&2; exit 1; }

# Our binaries and scripts.
install -d "$R/usr/local/bin" "$R/usr/local/sbin" "$R/lib"
for b in techo5 fbprobe audioprobe rebootto; do
	[ -e "$IN/bin/$b" ] && install -m 755 "$IN/bin/$b" "$R/usr/local/bin/$b"
done
install -m 755 "$IN/tools/slotctl" "$R/usr/local/sbin/slotctl"
install -m 644 "$IN/tools/techo5-lib.sh" "$R/lib/techo5-lib.sh"

# Overlay from the repo (tools/linux/rootfs), CRLF-safe.
say "overlay"
(cd "$IN/overlay" && tar -cf - .) | tar -xf - -C "$R"
find "$R/etc/techo5" "$R/usr/local/sbin" "$R/lib/techo5-lib.sh" "$R/etc/inittab" "$R/etc/profile.d/techo5.sh" "$R/etc/hostname" "$R/etc/motd" \
	-type f -exec sed -i 's/\r$//' {} +
chmod 755 "$R"/etc/techo5/*.sh "$R"/usr/local/sbin/*

# State that must survive an image change lives on userdata.
rm -rf "$R/etc/dropbear"
ln -s /data/techo5-linux/dropbear "$R/etc/dropbear"
rm -f "$R/etc/resolv.conf"
ln -s /run/resolv.conf "$R/etc/resolv.conf"

# Root: key-only SSH, no password.
install -d -m 700 "$R/root/.ssh"
install -m 600 "$IN/inputs/authorized_keys" "$R/root/.ssh/authorized_keys"
sed -i 's|^root:[^:]*:|root:*:|' "$R/etc/shadow"

# Clock.
if [ -e "$R/usr/share/zoneinfo/$TZNAME" ]; then
	ln -sf "/usr/share/zoneinfo/$TZNAME" "$R/etc/localtime"
	echo "$TZNAME" > "$R/etc/timezone"
else
	say "warning: no zoneinfo for $TZNAME; UTC"
fi

mkdir -p "$R/store" "$R/data" "$R/run" "$R/proc" "$R/sys" "$R/dev" "$R/tmp" "$R/newroot"
chmod 1777 "$R/tmp"
echo "techo5 rootfs $VERSION built $(date -u '+%Y-%m-%dT%H:%MZ') on $(cat /proc/sys/kernel/hostname), daemon $("$R/usr/local/bin/techo5" --version 2>/dev/null | head -1)" > "$R/etc/techo5-release"

say "packing"
rm -f "$OUT"
tar -czf "$OUT" -C "$R" .
say "$(cat "$R/etc/techo5-release")"
say "$OUT: $(du -h "$OUT" | cut -f1) ($(du -sh "$R" | cut -f1) unpacked)"
