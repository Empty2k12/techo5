# TECHO5

**Tech Echo 5** — an open, local-only firmware project for the Amazon Echo Show 5
(2nd generation, 2021, codename `cronos`).

The goal is a leaner and more capable replacement for the stock Alexa software:
a Home Assistant voice satellite with an on-device wake word, a native
`media_player`, and a dashboard display, with nothing leaving the house.

## Why

The Echo Show 5 is a 5.5-inch touchscreen, a far-field microphone array, a
speaker and a MediaTek MT8163 in a nice enclosure. Once the bootloader is
unlocked it is a small Linux computer that only needs three jobs done well:

1. hear a wake word and stream audio to Home Assistant Assist,
2. play speech and music,
3. show a dashboard.

Android is a lot of machinery for that. TECHO5 aims to do those jobs with a
small daemon and a kiosk, keep the hardware fully local, and stay easy to
update.

## Status

Early, but talking. The ported daemon (`echod/`) runs on a Show, appears in Home Assistant
as an ESPHome voice satellite, and completes voice turns: wake word on the device,
transcription and reply through Home Assistant, speech from the speaker. It is started by hand
on a bench unit with Android's framework stopped; see `docs/porting-plan.md` for what is left.

For daily use the working path on this hardware is still:

- bootloader unlock with [amonet-cronos](https://xdaforums.com/t/unlock-root-twrp-unbrick-amazon-echo-show-5-2nd-gen-2021-cronos.4772596/),
- [LineageOS 18.1 (unofficial)](https://xdaforums.com/t/rom-unofficial-11-cronos-lineageos-18-1-for-the-amazon-echo-show-5-2021.4772598/),
- the [ShowAssist](https://github.com/HuskerMinion/showassist) Android app as
  the satellite and display.

That works, but it is Android underneath. The plan in
[docs/porting-plan.md](docs/porting-plan.md) is to replace the Android layers
step by step, starting with the voice path.

## Installing on a Show

On a cronos unit already running LineageOS 18.1 with USB debugging and rooted debugging
enabled:

```powershell
cd echod
$env:GOOS='linux'; $env:GOARCH='arm'; $env:GOARM='7'; $env:CGO_ENABLED='0'
go build -trimpath -ldflags '-s -w' -o ../bin/echod-arm ./cmd/echod
cd ..
.\tools\install-cronos.ps1 -Serial <adb serial> -Name "Kitchen" -KeyFile .\kitchen.psk
```

The installer puts the daemon in place as an init service, switches Android to its null audio
HAL (the daemon owns the microphone and speaker), provisions the name, API key and wake word
models, and reboots. Home Assistant then discovers the device as an ESPHome node; paste the key
when asked. Android and any app on the screen keep running, silently.

## Approach

[EchoLocal](https://github.com/ygelfand/echolocal) (MIT) already turns the
Echo Dot 2 (`biscuit`, the same MT8163 family) into an ESPHome-native Home
Assistant satellite with a single static Go daemon that drives the hardware
directly. TECHO5 intends to bring that daemon to `cronos` and add the display
layer it does not have, rather than port a whole Linux distribution first.

See:

- [docs/hardware.md](docs/hardware.md) — what is known about the `cronos` hardware and the unlock path
- [docs/porting-plan.md](docs/porting-plan.md) — milestones and open questions

## Name

TECHO5 reads as "Tech Echo 5": the Echo hardware lineage and the 5.5-inch form
factor. Style it `TECHO5` or `tEcho5`.

## License

MIT. See [LICENSE](LICENSE).

TECHO5 is not affiliated with Amazon. Echo and Alexa are trademarks of
Amazon.com, Inc.
