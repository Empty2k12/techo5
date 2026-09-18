#!/usr/bin/env bash
# build-kernel-docker.sh, run build-kernel.sh inside a Linux container: the macOS path to a
# kernel, and anywhere else a container is easier than a cross toolchain.
#
#   DEVICE=checkers bash tools/linux/build-kernel-docker.sh -o inputs/Image.gz-dtb-checkers
#
# The kernel tree, the toolchain and the build directory live in a Docker volume rather than on
# the host: the Linux source carries files whose names differ only in case, which a stock macOS
# filesystem cannot hold. The repository is mounted read-only and only the built kernel comes
# back out. Arm ships the 8.3 toolchain build-kernel.sh wants for x86_64 hosts only, so on Apple
# silicon the compile runs emulated: slower, but unattended. The clone and the download do not,
# and should not: git's index-pack is what emulation breaks first.
#
# DEVICE, KCOMMIT and KPATCHED mean what they mean in build-kernel.sh. The first run clones the
# kernel (a few GB) and downloads the toolchain into the volume; later runs reuse both.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
export DEVICE=${DEVICE:-cronos}
export KCOMMIT=${KCOMMIT:-8d928c5176cc}
export KREPO=${KREPO:-https://github.com/amazon-oss/android_kernel_amazon_mt8163}
export KBRANCH=${KBRANCH:-cronos/lineage-18.1}
IMAGE=techo5-kbuild
VOL=techo5-kbuild
OUT=inputs/Image.gz-dtb-$DEVICE
while [ $# -gt 0 ]; do
	case "$1" in
	-o) OUT=$2; shift 2;;
	*) echo "unknown argument: $1" >&2; exit 1;;
	esac
done
mkdir -p "$(dirname "$OUT")"
OUTDIR=$(cd "$(dirname "$OUT")" && pwd)
export OUTNAME=$(basename "$OUT")

if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
	echo "== building the $IMAGE image"
	docker build --platform linux/amd64 -t "$IMAGE" - <<-'EOF'
	FROM ubuntu:22.04
	RUN apt-get update && apt-get install -y --no-install-recommends \
		bc bison build-essential ca-certificates curl flex git libssl-dev python3 xz-utils \
		&& rm -rf /var/lib/apt/lists/*
	EOF
fi

# The tree and the toolchain are fetched natively: only the compiler needs x86, and git's
# index-pack under emulation is what made the first clone die half way. A clone lands in a
# temporary directory and is moved into place whole, so an interrupted one leaves nothing behind.
docker run --rm -v "$VOL:/work" -e KREPO -e KBRANCH -e KCOMMIT alpine sh -euc '
	apk add --no-cache git curl tar xz >/dev/null
	if [ ! -x /work/toolchain/bin/aarch64-linux-gnu-gcc ]; then
		echo "== toolchain"
		rm -rf /work/toolchain && mkdir -p /work/toolchain
		curl -fL https://developer.arm.com/-/media/Files/downloads/gnu-a/8.3-2019.03/binrel/gcc-arm-8.3-2019.03-x86_64-aarch64-linux-gnu.tar.xz |
			tar -xJ -C /work/toolchain --strip-components=1
	fi
	if [ ! -d /work/kernel/.git ]; then
		echo "== kernel tree"
		rm -rf /work/kernel.new
		git -c pack.threads=1 -c pack.windowMemory=128m clone --filter=blob:none -b "$KBRANCH" "$KREPO" /work/kernel.new
		mv /work/kernel.new /work/kernel
	fi
	git -C /work/kernel rev-parse --verify --quiet "$KCOMMIT^{commit}" >/dev/null || git -C /work/kernel fetch origin
	git -C /work/kernel checkout -q --detach "$KCOMMIT"
'

docker run --rm --platform linux/amd64 \
	-v "$VOL:/work" -v "$ROOT:/src:ro" -v "$OUTDIR:/out" \
	-e DEVICE -e KCOMMIT -e KPATCHED -e OUTNAME \
	-e KSRC=/work/kernel -e KOUT=/work/out \
	-e CROSS_COMPILE=/work/toolchain/bin/aarch64-linux-gnu- \
	"$IMAGE" bash -euc 'bash /src/tools/linux/build-kernel.sh -o "/out/$OUTNAME"'
echo "built: $OUT"
