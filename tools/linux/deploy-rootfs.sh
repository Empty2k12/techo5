#!/usr/bin/env bash
# deploy-rootfs.sh — build the daemon and tools on the host, ship everything
# the device needs to build a root filesystem, build it there
# (tools/linux/mkrootfs.sh), and optionally install it into the inactive slot.
#
#   tools/linux/deploy-rootfs.sh [--install] [--reboot] [--version vX.Y.Z] [--on-device]
#
# The rootfs is built in WSL when it is there (fast: the host's CPU and disk,
# only the tarball crosses the Wi-Fi) and on the device otherwise or with
# --on-device. The WSL side needs qemu-user-static + binfmt-support (apt) and
# Alpine's static apk at ~/apk/apk.static; mkrootfs.sh runs inside a user
# namespace so files can be owned by root without sudo.
#
# Environment: HOST (192.168.1.50), KEY (the SSH key), TECHO5_INPUTS (the
# directory with the Alpine minirootfs, vendor.tar.gz, apks312/, and the
# public key; kept out of the repo), TZ_NAME (America/New_York), GO (go binary),
# WSL_DISTRO (Ubuntu). Git Bash on Windows is the expected shell.
set -euo pipefail

HOST=${HOST:-192.168.1.50}
KEY=${KEY:-D:/platform-tools/echoshow/linux-image/techo5_ed25519}
INPUTS=${TECHO5_INPUTS:-D:/platform-tools/echoshow/linux-image}
TZ_NAME=${TZ_NAME:-America/New_York}
GO=${GO:-/c/Program Files/Go/bin/go.exe}
VERSION=${VERSION:-}
WSL_DISTRO=${WSL_DISTRO:-Ubuntu}
INSTALL=; REBOOT=; ONDEVICE=
while [ $# -gt 0 ]; do
	case "$1" in
	--install) INSTALL=1; shift;;
	--reboot) REBOOT=1; shift;;
	--version) VERSION=$2; shift 2;;
	--on-device) ONDEVICE=1; shift;;
	*) echo "unknown argument: $1" >&2; exit 1;;
	esac
done

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
[ -n "$VERSION" ] || VERSION="dev-$(git -C "$ROOT" rev-parse --short HEAD)-$(date +%Y%m%d%H%M)"
STAGE=$ROOT/bin/rootfs-stage
SSH=(ssh -i "$KEY" -o StrictHostKeyChecking=no "root@$HOST")

echo "== building for armv7 ($VERSION)"
export GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0
pkg=github.com/HuskerMinion/techo5/echod/internal/layout
commit=$(git -C "$ROOT" rev-parse --short HEAD)
date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
(cd "$ROOT/echod" && "$GO" build -trimpath -ldflags "-s -w -X $pkg.Version=$VERSION -X $pkg.GitCommit=$commit -X $pkg.BuildDate=$date" -o "$ROOT/bin/echod-arm" ./cmd/echod)
for c in fbprobe audioprobe rebootto btbridge; do
	(cd "$ROOT" && "$GO" build -trimpath -ldflags "-s -w" -o "$ROOT/bin/$c-arm" "./cmd/$c")
done
unset GOOS GOARCH GOARM CGO_ENABLED

echo "== staging"
rm -rf "$STAGE"
mkdir -p "$STAGE/bin" "$STAGE/tools" "$STAGE/overlay" "$STAGE/inputs/apks312"
cp "$ROOT/bin/echod-arm" "$STAGE/bin/techo5"
for c in fbprobe audioprobe rebootto btbridge; do cp "$ROOT/bin/$c-arm" "$STAGE/bin/$c"; done
# The WebRTC echo canceller helper is C++ built separately (tools/linux/build-aec.sh in WSL); ship it when it is there.
[ -e "$ROOT/bin/techo5-aec-arm" ] && cp "$ROOT/bin/techo5-aec-arm" "$STAGE/bin/techo5-aec"
cp "$ROOT/tools/linux/slotctl" "$ROOT/tools/linux/techo5-lib.sh" "$ROOT/tools/linux/mkrootfs.sh" "$ROOT/tools/linux/packages-rootfs.txt" "$STAGE/tools/"
cp -r "$ROOT/tools/linux/rootfs/." "$STAGE/overlay/"
cp "$INPUTS"/alpine-minirootfs-*-armv7.tar.gz "$STAGE/inputs/"
cp "$INPUTS"/vendor/system-vendor-*.tar.gz "$STAGE/inputs/vendor.tar.gz"
cp "$INPUTS"/apks312/wpa_supplicant-2.9-*.apk "$INPUTS"/apks312/libssl1.1-*.apk "$INPUTS"/apks312/libcrypto1.1-*.apk "$STAGE/inputs/apks312/"
cp "$INPUTS/techo5_ed25519.pub" "$STAGE/inputs/authorized_keys"
# Wake word models ship in the image so a fresh unit answers to its default word; boot.sh copies
# them into the state directory when it is empty.
if [ -d "$INPUTS/models" ]; then mkdir -p "$STAGE/overlay/usr/share/techo5/models"; cp "$INPUTS"/models/*.tflite "$INPUTS"/models/*.json "$STAGE/overlay/usr/share/techo5/models/"; fi
# scripts must reach the device with LF endings whatever the checkout did
for f in "$STAGE"/tools/*.sh "$STAGE"/tools/slotctl "$STAGE"/tools/*.txt; do sed -i 's/\r$//' "$f"; done
find "$STAGE/overlay" -type f -exec sed -i 's/\r$//' {} +

OUT=/data/techo5-linux/techo5-rootfs-$VERSION.tar.gz
REMOTE=/data/techo5-linux/build
host_build=
if [ -z "$ONDEVICE" ] && command -v wsl.exe >/dev/null 2>&1 \
	&& wsl.exe -d "$WSL_DISTRO" -- bash -c 'test -e /proc/sys/fs/binfmt_misc/qemu-arm -a -x "$HOME/apk/apk.static"' 2>/dev/null; then
	host_build=1
fi

if [ -n "$host_build" ]; then
	echo "== building the rootfs in WSL ($WSL_DISTRO)"
	# Git Bash's /e/... is WSL's /mnt/e/...; no Windows path crosses over (wsl.exe eats backslashes).
	stage_wsl=$(cygpath -u "$STAGE" | sed 's|^/\([a-zA-Z]\)/|/mnt/\L\1/|')
	# The build itself lives in tools/linux/wsl-build.sh: one script, no quoting across wsl.exe.
	helper=$(cygpath -u "$ROOT/tools/linux/wsl-build.sh" | sed 's|^/\([a-zA-Z]\)/|/mnt/\L\1/|')
	tarball=$(MSYS_NO_PATHCONV=1 wsl.exe -d "$WSL_DISTRO" -- bash "$helper" "$stage_wsl" "$VERSION" "$TZ_NAME" | tr -d '\r' | tail -1)
	[ -n "$tarball" ] || { echo "WSL build failed" >&2; exit 1; }
	echo "== shipping the rootfs to $HOST"
	"${SSH[@]}" "mkdir -p /data/techo5-linux"
	scp -O -i "$KEY" -o StrictHostKeyChecking=no "$tarball" "root@$HOST:$OUT"
else
	echo "== shipping to $HOST"
	"${SSH[@]}" "rm -rf $REMOTE/in && mkdir -p $REMOTE/in"
	tar -czf - -C "$STAGE" . | "${SSH[@]}" "tar -xzf - -C $REMOTE/in"

	echo "== building the rootfs on the device"
	"${SSH[@]}" "sh $REMOTE/in/tools/mkrootfs.sh -i $REMOTE/in -o $OUT -w $REMOTE -V $VERSION -z $TZ_NAME"
fi

if [ -n "$INSTALL" ]; then
	echo "== installing into the inactive slot"
	"${SSH[@]}" "PATH=/usr/local/sbin:\$PATH; slotctl install $OUT && slotctl status"
	if [ -n "$REBOOT" ]; then
		echo "== rebooting"
		"${SSH[@]}" "sync; reboot" || true
	fi
else
	echo "built: $OUT (on the device). Install with: slotctl install $OUT"
fi
