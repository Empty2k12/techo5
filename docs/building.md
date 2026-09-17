# Building TECHO5 yourself

You don't need any of this to install or update a device. The installers and Home Assistant's update
card use the signed releases. This page is for changing the daemon or the images: what each build
needs, where every input comes from, and which script builds what.

The Echo Dot and Echo Spot have their own pages that build on this one:
[techo5-dot docs/building.md](https://github.com/HuskerMinion/techo5-dot/blob/main/docs/building.md) and
[techo5-spot docs/building.md](https://github.com/HuskerMinion/techo5-spot/blob/main/docs/building.md).

## What you need

| | Windows | Linux | macOS |
|---|---|---|---|
| The daemon (`echod`) and Go tools | [Go](https://go.dev/dl/) | Go | Go |
| Scripts in `tools/linux/*.sh` | Git Bash (from [Git for Windows](https://git-scm.com/download/win)) | bash | bash |
| Scripts in `tools/*.ps1` | [PowerShell 7](https://learn.microsoft.com/powershell/scripting/install/installing-powershell) (`pwsh`) | pwsh | pwsh |
| Image tools (`mkimage.py`, `patch-dtb.py`) | Python 3 (`pip install fdt` for `patch-dtb.py`) | Python 3 | Python 3 |
| Root filesystem | WSL (Ubuntu) with `qemu-user-static binfmt-support uidmap` | the same packages, or build on the device | build on the device |
| Kernel | WSL (Ubuntu) | Linux | a Linux VM or container |

The Go toolchain cross-compiles everything for the devices (`GOOS=linux GOARCH=arm GOARM=7`); no C
compiler is needed except for the kernel, bluez-alsa (Dot) and the optional echo canceller.

## Layout and environment

Nothing machine-specific is built in. Everything defaults to folders inside the checkout, all
git-ignored, and each can be moved with an environment variable:

| Variable | Default | What |
|---|---|---|
| `TECHO5_INPUTS` | `inputs/` | build inputs (below) |
| `KERNEL_IMAGE` | `inputs/boot-lineage-18.1-20260904-cronos.img` | your unit's LineageOS boot image |
| `KERNEL` | none | a rebuilt `Image.gz-dtb` to use instead of the one in `KERNEL_IMAGE` |
| `KEY` / `TECHO5_SSH_KEY` | `~/.ssh/id_ed25519` | the SSH key `deploy-rootfs.sh` uses to reach a unit |
| `GO` | `go` | the Go binary |
| `TECHO5_SIGN_KEY` | none | the release signing key (maintainer only) |

In WSL, pass Windows variables through with `WSLENV`, e.g. `setx WSLENV TECHO5_INPUTS/p`.

## Inputs

Everything public is fetched by one script, checked where a checksum exists:

```
pwsh ./tools/fetch-inputs.ps1 -Device show     # or -Device spot, or -Device dot -Dot ../techo5-dot
```

It fills `inputs/` with:

- `alpine-minirootfs-3.24.1-armv7.tar.gz`: Alpine's base image (pinned sha256);
- `busybox.static` (from Alpine's `busybox-static`) and `apk.static` (x86_64 `apk-tools-static` 2.14,
  for the root filesystem build; in WSL copy it to `~/apk/apk.static`);
- `apks/` and `apks312/`: the packages in [tools/linux/packages.txt](../tools/linux/packages.txt) for
  the rescue initramfs; the Wi-Fi drivers need wpa_supplicant 2.9 and its libraries, from Alpine 3.12;
- `models/`: the wake word models from
  [esphome/micro-wake-word-models](https://github.com/esphome/micro-wake-word-models) (`models/v2`).

Two inputs come from **your own unit** and are never published, because they are LineageOS and
vendor binaries:

- **The LineageOS boot image** (`inputs/boot-lineage-18.1-20260904-cronos.img`): with LineageOS
  running and adb as root, `adb pull /dev/block/by-name/boot`, or keep the `boot.img` from the
  LineageOS zip you installed. Its kernel and header are used as they are.
- **The vendor tree** (`inputs/vendor/system-vendor-cronos.tar.gz`): the Wi-Fi and Bluetooth
  modules, firmware and audio tuning from LineageOS's system partition. Take it before TECHO5
  replaces LineageOS:
  ```
  adb root
  adb shell "tar -czf /data/local/tmp/vendor.tgz -C /system vendor"
  adb pull /data/local/tmp/vendor.tgz inputs/vendor/system-vendor-cronos.tar.gz
  ```

## The daemon

```
cd echod
go test ./...
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -tags "" -o ../bin/echod-arm ./cmd/echod
```

Build tags pick the device: none for the Show 5, `dot` for the Echo Dot, `spot` for the Echo Spot.
To try a build on a running unit, turn on its SSH switch in Home Assistant (with a key sent through
the `ssh_keys` action), copy the binary over `/usr/local/bin/techo5` and restart the service; the
next update or a reboot into the other slot puts the release back.

## The root filesystem

[tools/linux/deploy-rootfs.sh](../tools/linux/deploy-rootfs.sh) builds the daemon and tools, stages
them with the inputs and [tools/linux/rootfs](../tools/linux/rootfs), and builds the tarball with
[mkrootfs.sh](../tools/linux/mkrootfs.sh) (Alpine packages from
[packages-rootfs.txt](../tools/linux/packages-rootfs.txt), installed by `apk.static` under QEMU):

```
HOST=<unit address> bash tools/linux/deploy-rootfs.sh --version v0.0.0-test            # build and copy it to the unit
HOST=<unit address> bash tools/linux/deploy-rootfs.sh --version v0.0.0-test --install   # and install it in the spare slot
```

On Windows it builds in WSL when WSL has `qemu-arm` binfmt and `~/apk/apk.static`, which is fast.
Anywhere else (Linux, macOS, or `--on-device`) it copies the stage to the unit over SSH and builds
there, which is slower but needs nothing on the computer beyond bash and Go.

## The kernel (Bluetooth)

LineageOS's kernel has no Bluetooth stack. [tools/linux/build-kernel.sh](../tools/linux/build-kernel.sh)
rebuilds it in WSL or Linux at the exact commit the LineageOS image came from, so the vendor modules
still load, with Bluetooth added; its header lists the source, the toolchain (Arm's GCC 8.3, no root
needed) and every variable. [tools/linux/README.md](../tools/linux/README.md) has the device tree edit
the Show needs (both microphones instead of their average).

## The boot image

```
bash tools/linux/build-image.sh -o bin/techo5-linux-boot.img            # with inputs/techo5_ed25519.pub for rescue SSH
bash tools/linux/build-image.sh -o bin/techo5-linux-boot.img --no-key   # as releases are built
```

`KERNEL=inputs/Image.gz-dtb-bt` uses the Bluetooth kernel. Flashing and the rest of the install are in
[install.md](install.md).

## The echo canceller (optional)

[tools/linux/build-aec.sh](../tools/linux/build-aec.sh) compiles the WebRTC echo canceller helper for
armv7 inside an Alpine root under QEMU (WSL or Linux). `deploy-rootfs.sh` includes `bin/techo5-aec-arm`
when it exists.

## Releases

[tools/release.ps1](../tools/release.ps1) (Show 5), `tools/release-dot.ps1` (Dot) and
`tools/release-spot.ps1` (Spot) sign a manifest with the key in `TECHO5_SIGN_KEY` and publish with
`gh`. Devices only take a manifest signed by the project's key, so a fork publishing its own releases
needs its own key and a daemon built with its public key.
