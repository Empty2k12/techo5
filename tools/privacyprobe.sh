#!/system/bin/sh
# Read-only look at the cronos privacy (mic mute) driver, LED platform devices and a few binaries. Run as root.
echo '--- gpio-privacy attrs:'
ls -la /sys/devices/platform/gpio-privacy/
for f in /sys/devices/platform/gpio-privacy/*; do
  [ -f "$f" ] && echo "$f = $(head -c 200 "$f" 2>&1 | tr '\n' ' ')"
done
echo '--- led@6:'
ls -la /sys/devices/platform/led@6/
ls /sys/devices/platform/led@6/leds 2>/dev/null
echo '--- leds-mt65xx:'
ls /sys/devices/platform/leds-mt65xx/
ls /sys/devices/platform/leds-mt65xx/leds 2>/dev/null
echo '--- devicetree gpio-privacy:'
ls /sys/firmware/devicetree/base/gpio-privacy/
for f in /sys/firmware/devicetree/base/gpio-privacy/*; do
  [ -f "$f" ] && echo "$f: $(od -An -tx1 "$f" 2>/dev/null | tr -d '\n' | head -c 160)"
done
echo '--- devicetree led@6:'
ls /sys/firmware/devicetree/base/led@6/ 2>/dev/null
for f in /sys/firmware/devicetree/base/led@6/*; do
  [ -f "$f" ] && echo "$f: $(head -c 80 "$f" 2>/dev/null | tr '\0' ' ')"
done
echo '--- stpbt / bt nodes:'
ls -l /dev/stpbt /dev/vhci /dev/ttyS* 2>&1 | head -5
echo '--- binaries:'
ls -l /system/bin/setprop /system/bin/iptables /system/bin/getprop /system/bin/chcon 2>&1
