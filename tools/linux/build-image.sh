#!/usr/bin/env bash
# build-image.sh — build the boot image (LineageOS kernel + the rescue/boot
# initramfs) with tools/linux/mkimage.py.
#
#   tools/linux/build-image.sh [-o out.img]
#
# Inputs come from TECHO5_INPUTS (D:/platform-tools/echoshow/linux-image):
# the LineageOS boot image, the Alpine minirootfs, busybox.static, the apks
# listed in packages.txt (apks/ and apks312/), and the SSH public key. The Go
# tools are built here. Git Bash on Windows is the expected shell.
set -euo pipefail

INPUTS=${TECHO5_INPUTS:-D:/platform-tools/echoshow/linux-image}
KERNEL_IMAGE=${KERNEL_IMAGE:-D:/platform-tools/echoshow/boot-lineage-18.1-20260904-cronos.img}
GO=${GO:-/c/Program Files/Go/bin/go.exe}
OUT=techo5-linux-boot.img
while [ $# -gt 0 ]; do
	case "$1" in
	-o) OUT=$2; shift 2;;
	*) echo "unknown argument: $1" >&2; exit 1;;
	esac
done
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
# Windows python wants Windows paths, and MSYS_NO_PATHCONV (needed so the
# x=/bin/y arguments survive) turns the automatic conversion off.
W() { cygpath -m "$1" 2>/dev/null || echo "$1"; }

echo "== building tools for armv7"
export GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0
for c in fbprobe audioprobe rebootto; do
	(cd "$ROOT" && "$GO" build -trimpath -ldflags "-s -w" -o "$ROOT/bin/$c-arm" "./cmd/$c")
done
unset GOOS GOARCH GOARM CGO_ENABLED

apks=()
for a in $(sed 's/#.*//' "$ROOT/tools/linux/packages.txt"); do
	case "$a" in
	busybox-static-*) continue;;
	wpa_supplicant-2.9*|libssl1.1*|libcrypto1.1*|libnl3-3.5*) apks+=(--apk "$(W "$INPUTS/apks312/$a")");;
	*) apks+=(--apk "$(W "$INPUTS/apks/$a")");;
	esac
done

echo "== mkimage"
export MSYS_NO_PATHCONV=1
R=$(W "$ROOT"); I=$(W "$INPUTS")
mini=$(ls "$INPUTS"/alpine-minirootfs-*-armv7.tar.gz | head -1)
python "$R/tools/linux/mkimage.py" --kernel-image "$(W "$KERNEL_IMAGE")" \
	--rootfs "$(W "$mini")" \
	"${apks[@]}" \
	--init "$R/tools/linux/init" \
	--add "$I/busybox.static=/bin/busybox.static" \
	--add "$R/bin/fbprobe-arm=/usr/local/bin/fbprobe" \
	--add "$R/bin/audioprobe-arm=/usr/local/bin/audioprobe" \
	--add "$R/bin/rebootto-arm=/usr/local/bin/rebootto" \
	--script "$R/tools/linux/slotctl=/usr/local/sbin/slotctl" \
	--script "$R/tools/linux/techo5-lib.sh=/lib/techo5-lib.sh" \
	--copy "$I/techo5_ed25519.pub=/root/.ssh/authorized_keys" \
	--compress xz --cmdline-append techo5=linux -o "$(W "$OUT")"
echo "built: $OUT"
echo "flash: fastboot flash boot $OUT && fastboot continue"
