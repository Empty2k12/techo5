#!/usr/bin/env bash
# deploy-rootfs.sh — build the daemon and tools on the host, ship everything
# the device needs to build a root filesystem, build it there
# (tools/linux/mkrootfs.sh), and optionally install it into the inactive slot.
#
#   tools/linux/deploy-rootfs.sh [--install] [--reboot] [--version vX.Y.Z]
#
# Environment: HOST (192.168.1.50), KEY (the SSH key), TECHO5_INPUTS (the
# directory with the Alpine minirootfs, vendor.tar.gz, apks312/, and the
# public key; kept out of the repo), TZ_NAME (America/New_York), GO (go binary).
# Git Bash on Windows is the expected shell.
set -euo pipefail

HOST=${HOST:-192.168.1.50}
KEY=${KEY:-D:/platform-tools/echoshow/linux-image/techo5_ed25519}
INPUTS=${TECHO5_INPUTS:-D:/platform-tools/echoshow/linux-image}
TZ_NAME=${TZ_NAME:-America/New_York}
GO=${GO:-/c/Program Files/Go/bin/go.exe}
VERSION=${VERSION:-}
INSTALL=; REBOOT=
while [ $# -gt 0 ]; do
	case "$1" in
	--install) INSTALL=1; shift;;
	--reboot) REBOOT=1; shift;;
	--version) VERSION=$2; shift 2;;
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
cp "$ROOT/tools/linux/slotctl" "$ROOT/tools/linux/techo5-lib.sh" "$ROOT/tools/linux/mkrootfs.sh" "$ROOT/tools/linux/packages-rootfs.txt" "$STAGE/tools/"
cp -r "$ROOT/tools/linux/rootfs/." "$STAGE/overlay/"
cp "$INPUTS"/alpine-minirootfs-*-armv7.tar.gz "$STAGE/inputs/"
cp "$INPUTS"/vendor/system-vendor-*.tar.gz "$STAGE/inputs/vendor.tar.gz"
cp "$INPUTS"/apks312/wpa_supplicant-2.9-*.apk "$INPUTS"/apks312/libssl1.1-*.apk "$INPUTS"/apks312/libcrypto1.1-*.apk "$STAGE/inputs/apks312/"
cp "$INPUTS/techo5_ed25519.pub" "$STAGE/inputs/authorized_keys"
# scripts must reach the device with LF endings whatever the checkout did
for f in "$STAGE"/tools/*.sh "$STAGE"/tools/slotctl "$STAGE"/tools/*.txt; do sed -i 's/\r$//' "$f"; done
find "$STAGE/overlay" -type f -exec sed -i 's/\r$//' {} +

echo "== shipping to $HOST"
REMOTE=/data/techo5-linux/build
"${SSH[@]}" "rm -rf $REMOTE/in && mkdir -p $REMOTE/in"
tar -czf - -C "$STAGE" . | "${SSH[@]}" "tar -xzf - -C $REMOTE/in"

OUT=/data/techo5-linux/techo5-rootfs-$VERSION.tar.gz
echo "== building the rootfs on the device"
"${SSH[@]}" "sh $REMOTE/in/tools/mkrootfs.sh -i $REMOTE/in -o $OUT -w $REMOTE -V $VERSION -z $TZ_NAME"

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
