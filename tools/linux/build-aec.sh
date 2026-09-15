#!/bin/bash
# build-aec.sh — compile tools/aec/techo5-aec.cpp for armv7 inside an Alpine root under
# QEMU user emulation, in WSL. The first run makes the root (~/alpine-armv7-sdk) from the
# minirootfs and installs the compiler and webrtc-audio-processing; later runs reuse it.
#
#   tools/linux/build-aec.sh [-o bin/techo5-aec-arm]
#
# Needs what wsl-build.sh needs (qemu-user-static, binfmt-support, uidmap, ~/apk/apk.static)
# and the Alpine minirootfs from TECHO5_INPUTS (default D:\platform-tools\echoshow\linux-image).
set -euo pipefail
# The repo: from this script's place, or TECHO5_ROOT when the script was copied elsewhere (deploy
# copies it to a Linux path so WSL's bash does not trip over CRLF).
ROOT=${TECHO5_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}
export TECHO5_ROOT=$ROOT
INPUTS=${TECHO5_INPUTS:-/mnt/d/platform-tools/echoshow/linux-image}
SDK=$HOME/alpine-armv7-sdk
APK=$HOME/apk/apk.static
OUT=$ROOT/bin/techo5-aec-arm
while [ $# -gt 0 ]; do
	case "$1" in
	-o) OUT=$2; shift 2;;
	*) echo "unknown argument: $1" >&2; exit 1;;
	esac
done

if [ -z "${TECHO5_IN_NS:-}" ]; then
	exec unshare -Ur --map-auto env TECHO5_IN_NS=1 bash "$0" -o "$OUT"
fi

if [ ! -x "$SDK/usr/bin/g++" ]; then
	echo "== making the armv7 SDK root at $SDK"
	rm -rf "$SDK"; mkdir -p "$SDK"
	tar -xzf "$(ls "$INPUTS"/alpine-minirootfs-*-armv7.tar.gz | head -1)" -C "$SDK"
	cp /etc/resolv.conf "$SDK/etc/resolv.conf"
	"$APK" --root "$SDK" --arch armv7 --no-cache add build-base webrtc-audio-processing-1-dev pkgconf
fi

mkdir -p "$SDK/build"
cp "$ROOT/tools/aec/techo5-aec.cpp" "$SDK/build/"
echo "== compiling under QEMU"
chroot "$SDK" /bin/sh -c 'cd /build && g++ -O2 -std=c++17 -o techo5-aec techo5-aec.cpp $(pkg-config --cflags --libs webrtc-audio-processing-1) && strip techo5-aec && ls -la techo5-aec'
mkdir -p "$(dirname "$OUT")"
cp "$SDK/build/techo5-aec" "$OUT"
echo "built: $OUT"
