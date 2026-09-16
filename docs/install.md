# Installing TECHO5 on an Echo Show 5

Step by step, from an Echo Show 5 (2nd generation, `cronos`) running LineageOS to one running
TECHO5. Written from a first install on a second unit (2026-09-16); every command here was run on
that install. Placeholders: `<serial>` is the unit's adb/fastboot serial, `<address>` its IP address
on your network, `<version>` a release such as `v0.2.7`.

**This erases Android.** LineageOS on the `system` partition is replaced by the TECHO5 slot store.
`userdata` is kept (TECHO5 reads the Wi-Fi network Android saved from it), and TWRP stays in
`recovery`, so LineageOS can be put back with TWRP and its zip.

## What you need

- An Echo Show 5 2nd gen already unlocked with
  [amonet-cronos](https://xdaforums.com/t/unlock-root-twrp-unbrick-amazon-echo-show-5-2nd-gen-2021-cronos.4772596/)
  and running
  [LineageOS 18.1](https://xdaforums.com/t/rom-unofficial-11-cronos-lineageos-18-1-for-the-amazon-echo-show-5-2021.4772598/),
  connected to your Wi-Fi in Android, with USB debugging on.
- Its power adapter and a USB **data** cable to the PC. Keep it on mains power while flashing.
- A Windows PC with Git Bash, `adb` and `fastboot` (Android platform tools), and a serial terminal
  such as PuTTY. Go, Python 3 and WSL (Ubuntu) only if you build the boot image yourself.
- Home Assistant with the ESPHome integration.

Work in Git Bash. It rewrites arguments that look like paths, including paths on the device, so set
`export MSYS_NO_PATHCONV=1` first or `adb push … /sdcard/…` lands somewhere else.

If more than one Android or fastboot device is plugged in, pass `-s <serial>` to every `adb` and
`fastboot` command and check `adb devices -l` first.

## 1. Get the boot image

The boot image is the kernel plus the small rescue environment that sets a unit up. Two ways:

- **From a release (simplest).** Download `techo5-boot-<version>.img` from the same
  [release](https://github.com/HuskerMinion/techo5/releases) as the root filesystem. It carries no
  SSH key, so steps 4 and 5 are typed into the unit's **USB serial console**.
- **Built yourself with your own SSH key**, to SSH into the rescue environment instead:

  ```
  ssh-keygen -t ed25519 -f techo5_ed25519        # into your inputs directory
  ```

  then build the kernel and boot image as [tools/linux/README.md](../tools/linux/README.md) describes
  ("Build": inputs, `build-kernel.sh` in WSL, `patch-dtb.py`, `build-image.sh`).

Either way the kernel is the LineageOS commit the Show's own kernel came from, so the vendor Wi-Fi
and Bluetooth modules load. Check before flashing:

```
adb -s <serial> shell uname -r      # must be 4.9.337-g8d928c5176cc
```

## 2. Put the root filesystem on the unit

Download `techo5-rootfs-<version>.tar.gz` from the latest
[release](https://github.com/HuskerMinion/techo5/releases) and copy it to the unit's storage, which
TECHO5 can read from its rescue environment:

```
adb -s <serial> push techo5-rootfs-<version>.tar.gz /sdcard/Download/
adb -s <serial> shell md5sum /sdcard/Download/techo5-rootfs-<version>.tar.gz   # compare with your copy
```

## 3. Flash the boot image

```
adb -s <serial> reboot bootloader
fastboot devices                                  # exactly the unit you mean
fastboot -s <serial> flash boot techo5-boot-<version>.img    # or your own techo5-linux-boot.img
fastboot -s <serial> continue
```

The unit boots the TECHO5 initramfs. With LineageOS still on `system` there is no slot store, so it
stays in the **rescue environment**: it joins the Wi-Fi network Android saved and shows a test
screen. Open a root shell on it:

- **USB serial console** (any boot image): the unit appears in Device Manager as a new "USB Serial
  Device (COMn)". Open that port in PuTTY (connection type Serial, speed 115200) and press Enter for
  a `#` prompt.
- **SSH** (a boot image built with your key), within about a minute:

  ```
  ssh -i techo5_ed25519 root@<address>
  ```

## 4. Create the slot store and install

In the rescue shell. `mkstore` is the step that erases LineageOS.

```
cat /proc/idme/serial                             # the unit you mean
umount /android                                   # LineageOS's system, mounted read-only
PATH=/usr/local/sbin:$PATH
slotctl mkstore /dev/mmcblk0p12 --i-know-this-erases-it
STORE=/store slotctl install /data/media/0/Download/techo5-rootfs-<version>.tar.gz
STORE=/store slotctl status                       # slot a: trial 3
```

If `mkfs failed` mentions `libgcc_s.so.1`, the boot image predates the fix (one older than v0.2.8):
take the library from the root filesystem and run `mkstore` again.

```
tar xzf /data/media/0/Download/techo5-rootfs-<version>.tar.gz -C /tmp ./usr/lib/libgcc_s.so.1
cp /tmp/usr/lib/libgcc_s.so.1 /usr/lib/
```

## 5. Provision before the first boot

Still in the rescue shell. None of this is required, but each saves a step later.

```
mkdir -p /data/misc/techo5
printf 'Kitchen\n' > /data/misc/techo5/name       # the name Home Assistant shows

# The ESPHome encryption key. Keep the printed value for Home Assistant.
umask 077; head -c 32 /dev/urandom | base64 > /data/misc/techo5/psk; cat /data/misc/techo5/psk

# Only with a boot image built with your key: keep SSH after the switch to the slot. Otherwise
# skip these four lines; SSH stays off, and a key comes later from Home Assistant (ssh_keys).
mkdir -p -m 700 /data/misc/techo5/ssh
cp /root/.ssh/authorized_keys /data/misc/techo5/ssh/authorized_keys
chmod 600 /data/misc/techo5/ssh/authorized_keys
printf '{"security":{"ssh":true}}\n' > /data/misc/techo5/state.json   # only on a unit with no state.json yet
sync
```

## 6. Boot TECHO5

The rescue environment's PID 1 is a script, so a plain `reboot` does nothing:

```
/bin/busybox.static reboot -f
```

The first boot after `adb reboot bootloader` may stop at the bootloader ("hacked fastboot") once.
Continue it from the PC:

```
fastboot -s <serial> continue
```

The unit boots slot a, the daemon starts, and after five minutes of running the slot commits
(`slotctl status`: `good`).

## 7. Add it to Home Assistant

Settings → Devices & services: the unit appears under Discovered as an ESPHome device with the name
from step 5. Add it and paste the key printed in step 5 when asked. If it does not appear, add the
ESPHome integration by hand with host `<address>` and port 6053.

Then:

- **Time zone**: nothing to set. The unit starts on UTC and takes Home Assistant's zone as soon as it
  connects, and keeps it from then on.
- **Wake word**: the default is "Alexa". Change it on the device (swipe down from the top, Device tab,
  Wake word, Next) or in Home Assistant; the other follows.
- **Security**: SSH, and the camera and screen pages on port 8181, have switches on the Security tab
  and in Home Assistant. SSH keys only come from Home Assistant (`esphome.<device>_ssh_keys`).
- **Updates**: the firmware update entity installs new releases into the other slot, reboots, and
  falls back if the new slot does not settle.
- The old Android integrations for the unit (ShowAssist, the View Assist companion) can be deleted.

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| The daemon restarts every few seconds on a fresh install, log shows `slice bounds out of range` in `microwakeword` | v0.2.5 shipped damaged wake word models. Use v0.2.6 or later; on a unit already installed, copy good `.tflite` files over `/data/misc/techo5/models/`. |
| An update from Home Assistant fails with `context deadline exceeded` | The download was too slow, usually on 2.4 GHz next to the unit's own Bluetooth. Press Install again; from v0.2.5 the unit moves itself to the network's 5 GHz radio when one is in range. |
| The screen shows "hacked fastboot" | A leftover `reboot bootloader` request: `fastboot -s <serial> continue`. |
| No SSH after the switch to the slot | SSH is off unless step 5 was done: turn on the SSH switch in Home Assistant and send a key with the `ssh_keys` action, or use the USB serial console. |
| The unit sits in rescue | No bootable slot. `slotctl status` shows why; the daemon still runs from a slot in rescue, so Home Assistant keeps working while you look. |

To go back to LineageOS: boot TWRP from `recovery`, flash the LineageOS zip (which rewrites `system`)
and the LineageOS boot image.
