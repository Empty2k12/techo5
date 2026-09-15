#!/system/bin/sh
# Bench mode without the Android framework: kernel, Wi-Fi, adb and the daemon only.
# Usage (as root): sh /data/local/tmp/bench-nofw.sh
#
# Stopping zygote takes system_server, the launcher and the boot animation down and leaves the
# native services. Android's connectivity service removes the policy-routing rules that select
# the Wi-Fi table when it dies, so this puts one back. `reboot` is the way out.
D=/data/local/tmp
for p in $(pidof echod); do kill "$p"; done
setprop ctl.stop zygote
sleep 3
setprop ctl.stop audioserver
setprop ctl.stop bootanim
setprop ctl.stop vendor.audio-hal
sleep 2
echo "zygote=$(getprop init.svc.zygote) audioserver=$(getprop init.svc.audioserver) hal=$(getprop init.svc.vendor.audio-hal)"
# The Wi-Fi network's own table still holds the routes; make lookups reach it.
T=$(ip route show table all | grep -E "^default via .* dev wlan0 table" | sed -E 's/.* table ([0-9]+).*/\1/' | head -1)
if [ -n "$T" ]; then
  ip rule add from all lookup "$T" pref 5000 2>/dev/null
  echo "routing: rule -> table $T"
else
  ip route add default via 192.168.1.1 dev wlan0 2>&1
  echo "routing: default route in main"
fi
echo "ha: $(curl -s -m 5 -o /dev/null -w %{http_code} http://192.168.1.20:8123/)"
echo "--- holders before start:"
sh $D/holders.sh
mkdir -p /data/misc/techo5 /data/techo5
chmod 755 $D/echod
logcat -c
cd $D && (nohup ./echod run > $D/echod.log 2>&1 &)
sleep 1
echo "echod pid: $(pidof echod)"
echo "mem: $(grep MemAvailable /proc/meminfo)"
