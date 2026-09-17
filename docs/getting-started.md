# Getting started

From a stock Amazon Echo to one running TECHO5, step by step, for each device TECHO5 supports. Each
step links to the guide that does it; this page is the order to do them in, and what to check before
moving on.

> **Read this first.** Unlocking an Echo uses bootloader exploits. It can brick the device, it voids
> whatever warranty is left, and it wipes the device's data. Do it on a unit you can afford to lose,
> keep it on mains power the whole time, and never unplug it while anything is being written. These
> are hobby projects, not products.

## Which Echo do you have?

The model number is on the bottom of the device, or in the Alexa app under the device's settings.

| Device | Codename | Model | Supported by | Difficulty today |
|---|---|---|---|---|
| **Echo Show 5, 2nd gen** (2021) | `cronos` | AEOCN | [TECHO5](https://github.com/HuskerMinion/techo5) | Moderate: prebuilt images, a guided manual install |
| **Echo Dot, 2nd gen** (2016) | `biscuit` | RS03QR | [TECHO5 Dot](https://github.com/HuskerMinion/techo5-dot) | Moderate: the unlock and Fire OS steps by hand, then a one-command installer |
| **Echo Spot, 1st gen** (2017) | `rook` | VN94DQ | [TECHO5 Spot](https://github.com/HuskerMinion/techo5-spot) | Moderate: the unlock and LineageOS by hand, then a one-command installer |

Other Echos (the Show 5 1st gen, the Dot 3rd gen and later, the Show 8, and so on) are **not**
supported.

## What every device needs

- A **USB data cable** for the device's USB port (micro-USB on the Dot and the Spot). A charge-only
  cable won't work.
- **Home Assistant** with the ESPHome integration (built in).
- A computer. Which one depends on the device and the step; see [Windows, Linux or macOS](#windows-linux-or-macos)
  below.
- For the Dot and Spot installers: [PowerShell 7](https://learn.microsoft.com/powershell/scripting/install/installing-powershell)
  (`pwsh`), the Android platform tools (`adb`, `fastboot`), Python 3, and `git` to fetch the repository.
  Nothing is compiled: the installers download the signed release (root filesystem, Bluetooth kernel
  and rescue packages), check every file against its checksum, and build the boot image from your own
  unit's backup.
- The unlock threads on XDA need a (free) XDA account to download attachments.

## Echo Show 5 (2nd gen)

1. **Unlock it with amonet-cronos.** Follow
   [[UNLOCK][ROOT][TWRP][UNBRICK] Echo Show 5 2nd Gen (cronos)](https://xdaforums.com/t/unlock-root-twrp-unbrick-amazon-echo-show-5-2nd-gen-2021-cronos.4772596/)
   on XDA (source: [R0rt1z2/amonet, branch mt8163-cronos](https://github.com/R0rt1z2/amonet/tree/mt8163-cronos)).
   In short: with the Show on mains power, hold all three buttons until the screen says
   `=> FASTBOOT mode`, connect USB, and run the fastbrick step from the thread. It reboots into TWRP
   on its own; don't interrupt it.
   *Check:* the Show boots into TWRP.
2. **Install LineageOS 18.1.** Follow
   [[ROM][UNOFFICIAL][11][cronos] LineageOS 18.1 for the Echo Show 5 (2021)](https://xdaforums.com/t/rom-unofficial-11-cronos-lineageos-18-1-for-the-amazon-echo-show-5-2021.4772598/).
   Use a current build (0.4 or later; earlier ones lose audio after a few days).
   *Check:* LineageOS boots.
3. **Prepare LineageOS:** join your Wi-Fi, then in Settings → About → tap Build number seven times,
   and in Developer options turn on **USB debugging**.
   *Check:* `adb devices` on the computer lists the Show as `device`.
4. **Install TECHO5.** Follow [docs/install.md](install.md). The
   [latest release](https://github.com/HuskerMinion/techo5/releases/latest) has the boot image and
   the root filesystem, so nothing needs building.
   *Check:* the screen shows the TECHO5 clock.
5. **Add it to Home Assistant**: see [After installing](#after-installing-every-device).

## Echo Dot (2nd gen)

These steps follow [proffalken's write-up](https://gist.github.com/proffalken/377ae50146affe1886dddaaacb87926b)
of installing TECHO5 Dot from Linux, which found the exact Fire OS build that avoids SELinux boot
loops. Every command runs the same on Windows, Linux and macOS once the Dot is unlocked.

1. **Unlock it with amonet-biscuit.** Follow
   [[UNLOCK][ROOT][TWRP][UNBRICK] Echo Dot 2nd Gen / 2016 (biscuit)](https://xdaforums.com/t/unlock-root-twrp-unbrick-amazon-echo-dot-2nd-gen-2016-biscuit.4761416/)
   (source: [R0rt1z2/amonet, branch mt8163-biscuit](https://github.com/R0rt1z2/amonet/tree/mt8163-biscuit)).
   Use the release ZIP attached to the thread, not the bare git repository: it has `fastbrick.sh`,
   `boot-root.zip` and the files they need. The unlock needs a **Linux** computer or live USB.
   - Hold the **Action** button while plugging in power; the light turns **green** (Amazon's factory
     fastboot mode). Connect USB.
   - Run `./fastbrick.sh`, check the device it found, and type `YES`. It ends with
     `Exploit most likely successful!` and reboots into TWRP (a **white** light).
   - To get back into TWRP later: unplug power, hold **Volume Up**, plug power back in, and wait for the
     white light.

   *Check:* `adb devices` lists the Dot as `recovery`.
2. **Flash Fire OS 6574.1 into both slots.** TECHO5 Dot's Bluetooth kernel is built from this exact
   build's source (NS6574, build 7623), and its Wi-Fi driver is loaded from Fire OS's system partition,
   so other builds don't match. The file is
   `update-kindle-biscuit_puffin-NS6574_user_7623_0013121734532.bin` (the XDA thread links Amazon's
   update files). From TWRP:
   ```
   adb shell twrp wipe cache
   adb shell twrp wipe data
   adb push update-kindle-biscuit_puffin-NS6574_user_7623_0013121734532.bin /sdcard/update.zip
   adb shell twrp install /sdcard/update.zip
   adb reboot recovery
   adb shell twrp install /sdcard/update.zip
   ```
   Installing twice puts it in both A/B slots. A "no selinux policy bundled" warning in the install log
   means the build doesn't match; stop and get the right file.
3. **Root it.** Still in TWRP:
   ```
   adb push boot-root.zip /sdcard/
   adb shell twrp install /sdcard/boot-root.zip
   ```
4. **Trust this computer's adb key**, so Fire OS never needs to show an authorization prompt (the Dot
   has no screen to show it on):
   ```
   adb push ~/.android/adbkey.pub /sdcard/adbkey.pub
   adb shell "mkdir -p /data/misc/adb && cp /sdcard/adbkey.pub /data/misc/adb/adb_keys && chown 1000:2000 /data/misc/adb/adb_keys && chmod 640 /data/misc/adb/adb_keys"
   adb shell restorecon -v /data/misc/adb/adb_keys
   adb reboot
   ```
   On Windows the key is `%USERPROFILE%\.android\adbkey.pub`. (Run `adb devices` once first if the
   file doesn't exist yet.)
5. **Join Wi-Fi once in Fire OS.** Complete the Alexa app's Wi-Fi step; skipping the rest of the Alexa
   setup is fine. The installer reads the saved network. (Skip this and it asks for a network instead.)
   *Check:* `adb devices` lists the Dot as `device`, and `adb shell id` says `uid=0`.
6. **Install TECHO5 Dot.**
   ```
   git clone https://github.com/HuskerMinion/techo5-dot
   cd techo5-dot
   pwsh ./tools/install-dot.ps1 -Serial <serial> -DryRun
   pwsh ./tools/install-dot.ps1 -Serial <serial> -Name "Kitchen"
   ```
   `<serial>` is what `adb devices` shows. The dry run checks the Dot, backs up every partition that
   boots it into `backups/<serial>/` (keep that folder: it's the way back), downloads and checks the
   release, and builds this Dot's boot image, writing nothing to the Dot. The second run installs, with
   Bluetooth, and waits for the first boot to report healthy. (The installer also works straight from
   TWRP after step 2, without steps 3 to 5.)
   *Check:* the installer ends with the Dot healthy and its Home Assistant port answering.
7. **Add it to Home Assistant**: see [After installing](#after-installing-every-device).

Prefer to keep Fire OS? [EchoLocal](https://github.com/ygelfand/echolocal), the project TECHO5's daemon
is built on, runs on the unlocked Dot's Fire OS 6 with its own installer, and is the gentler path.

## Echo Spot (1st gen)

1. **Check the Fire OS version.** amonet-rook supports Fire OS **5.5.6.9, 5.5.5.2 and 5.5.3.4** only.
   On the Spot: Settings → Device Options → Device Software Version. If it's older, let it update
   first.
2. **Unlock it with amonet-rook.** Follow
   [[UNLOCK][ROOT][TWRP][UNBRICK] Echo Spot 2017 (rook)](https://xdaforums.com/t/unlock-root-twrp-unbrick-amazon-echo-spot-2017-rook.4754878/)
   (source: [R0rt1z2/amonet, branch mt8163-rook](https://github.com/R0rt1z2/amonet/tree/mt8163-rook)).
   Everything goes through the micro-USB port under the back cover. The bench unit was unlocked from
   Windows with the thread's `fastbrick` step (fastboot: hold Volume Up, Volume Down and Mute at
   power-on); the thread also has the Linux route. Plan for the Spot's data to be wiped.
   *Check:* the Spot boots into TWRP.
3. **Install LineageOS 18.1.** Follow
   [[ROM][UNOFFICIAL][11][rook] LineageOS 18.1 for the Echo Spot (2017)](https://xdaforums.com/t/rom-unofficial-11-rook-lineageos-18-1-for-the-amazon-echo-spot-2017.4762459/).
   *Check:* LineageOS boots.
4. **Prepare LineageOS:** join your Wi-Fi, and in Developer options (tap Build number seven times to
   show them) turn on **USB debugging** and **Rooted debugging**; the installer needs adb as root.
   If the on-screen keyboard won't type digits in the Wi-Fi password, add the network from the
   computer with `adb shell cmd wifi connect-network "<network>" wpa2 "<password>"`.
5. **Back it up** from TWRP (`adb reboot recovery` from LineageOS), with the Spot connected by USB:
   ```
   git clone https://github.com/HuskerMinion/techo5-spot
   cd techo5-spot
   pwsh ./tools/backup-spot.ps1 -Serial <serial> -IncludeSystem
   ```
   Keep `backups/<serial>/`: it's the way back to LineageOS and Fire OS.
6. **Install TECHO5 Spot**, booted back into LineageOS with rooted debugging on:
   ```
   pwsh ./tools/install-spot-linux.ps1 -Serial <serial> -Name "Kitchen" -BuildOnly
   pwsh ./tools/install-spot-linux.ps1 -Serial <serial> -Name "Kitchen"
   ```
   The first run captures what it needs from this Spot, downloads and checks the release, and builds
   the boot image, touching nothing else. The second replaces LineageOS with TECHO5 (it asks before
   erasing), with Bluetooth, and waits for the first boot. `-Logo` also replaces the bootloader's
   Amazon picture (needs `pip install pillow`).
   *Check:* the round screen shows the TECHO5 clock.
7. **Add it to Home Assistant**: see below.

## After installing (every device)

1. **Add it.** Home Assistant discovers it as an ESPHome device (Settings → Devices & services).
   Paste the encryption key the installer printed.
2. **Allow it to perform Home Assistant actions.** On the device's ESPHome entry → Configure, turn on
   **Allow the device to perform Home Assistant actions**. The radio favorites, phone call events and
   some screen features need it.
3. **Pick a wake word** on the device's Assist satellite (Okay Nabu, Hey Jarvis, Hey Mycroft, and
   Alexa on the Show and Spot).
4. **Updates** from then on come from Home Assistant's update card: signed releases install into the
   spare slot and roll back on their own if they don't come up healthy.
5. **Optional:**
   - [phone calls](phone.md) through your own SIP provider;
   - SSH (keys only) through the `ssh_keys` action and the SSH switch;
   - cameras, radio lists and weather from the device's Home Assistant actions (see each README);
   - multi-room audio through [Music Assistant](https://www.music-assistant.io/) (each device is a
     Sendspin player).

## Windows, Linux or macOS

| Step | Windows | Linux | macOS |
|---|---|---|---|
| Unlock: Show 5 (amonet-cronos) | Yes (fastbrick) | Yes | Use a Linux live USB |
| Unlock: Dot (amonet-biscuit) | Use a Linux live USB | **Yes** (what the thread uses) | Use a Linux live USB |
| Unlock: Spot (amonet-rook) | Yes (fastbrick, as on the bench unit) | Yes | Use a Linux live USB |
| LineageOS (Show 5, Spot) | Yes | Yes | Yes (TWRP and `adb` only) |
| Install TECHO5 on the Show 5 ([install.md](install.md)) | Yes (Git Bash, PuTTY) | **Yes** | **Yes** |
| Fire OS 6574.1, root, adb key (Dot) | Yes | Yes | Yes |
| Install TECHO5 Dot (`install-dot.ps1`) | **Yes** (pwsh) | **Yes** (pwsh) | **Yes** (pwsh) |
| Install TECHO5 Spot (`install-spot-linux.ps1`) | **Yes** (pwsh) | **Yes** (pwsh) | **Yes** (pwsh) |
| Updates after that | Home Assistant | Home Assistant | Home Assistant |

**On Linux:**
- Install the Android platform tools (`adb` and `fastboot`, e.g. `sudo apt install adb fastboot`)
  and a serial terminal (`screen` or `picocom`).
- For the Show 5's install.md, the Git Bash commands run as they are in any Linux shell; skip
  `MSYS_NO_PATHCONV`, and open the USB serial console with `screen /dev/ttyACM0 115200` instead of
  PuTTY. You may need to be in the `dialout` group (`sudo usermod -aG dialout $USER`, then log in
  again).
- If ModemManager is installed, stop it while working with serial consoles and the BootROM
  (`sudo systemctl stop ModemManager`); it grabs new USB serial ports.

**On macOS:**
- Install the platform tools with Homebrew (`brew install android-platform-tools`); `screen` is
  built in: `screen /dev/tty.usbmodem* 115200`.
- The amonet BootROM steps need Linux. A live USB (Ubuntu) is more reliable than a virtual machine:
  the exploit re-enumerates USB mid-way, and VM USB passthrough often loses the device.
- The Show 5's install.md works from Terminal once the device is unlocked and on LineageOS.

**The Dot and Spot installers** run in PowerShell 7 on all three (`sudo snap install powershell --classic`
on Ubuntu, `brew install powershell` on macOS, `winget install Microsoft.PowerShell` on Windows) and
find the device's USB serial console on each. Nothing is built on your computer. Building the images
yourself instead is described in each repository's `docs/building.md`.

## If something goes wrong

- **Before TECHO5 is installed:** each XDA thread has an unbrick section. Don't improvise with
  bootloader images; that is how Echos get hard-bricked.
- **After:** every TECHO5 device keeps TWRP, and falls back to its previous slot, and then to a
  rescue environment with a USB serial console, if a boot doesn't come up healthy. Each
  repository's docs describe the way back to LineageOS or Fire OS.
- Ask in the project's GitHub issues, with the device, the step and what it printed.

## Credits

The unlocks are the work of [R0rt1z2](https://github.com/R0rt1z2) and k4y0z (amonet, kaeru and the
Echo TWRP builds); LineageOS for these devices is R0rt1z2's and
[amazon-oss](https://github.com/amazon-oss)'s. TECHO5's daemon is built on
[EchoLocal](https://github.com/ygelfand/echolocal) by Yuri Gelfand. The Echo Dot steps come from
[proffalken](https://github.com/proffalken)'s
[write-up](https://gist.github.com/proffalken/377ae50146affe1886dddaaacb87926b) of installing TECHO5
Dot from Linux, whose fixes also made the installers cross-platform.
