#!/system/bin/sh
# Bench helper: restart the daemon from /data/local/tmp, killing any running copy first.
# Usage (as root): sh /data/local/tmp/bench-restart.sh
#
# Assumes Android is on the null audio HAL (ro.hardware.audio.primary=default in
# /system/build.prop), so audioserver never opens the PCM devices and the framework, ShowAssist
# and the daemon run side by side. With the Amazon HAL selected instead, audioserver crash-loops
# against the daemon and takes system_server with it — see docs/hardware.md.
D=/data/local/tmp
for p in $(pidof echod); do kill "$p"; done
sleep 1
setprop ctl.stop techo5 2>/dev/null
echo "audio HAL: $(getprop ro.hardware.audio.primary)"
echo "--- holders before start:"
sh $D/holders.sh
mkdir -p /data/misc/techo5 /data/techo5
chmod 755 $D/echod
logcat -c
cd $D && (nohup ./echod run > $D/echod.log 2>&1 &)
sleep 1
echo "echod pid: $(pidof echod)"
