#!/system/bin/sh
# Bench helper: stop everything that holds the audio devices, then start the daemon fresh.
# Usage (as root): sh /data/local/tmp/bench-restart.sh [nohal]
#   nohal  - also stop the vendor audio HAL (Android audio stays down until `setprop ctl.start vendor.audio-hal`)
D=/data/local/tmp
for p in $(pidof echod); do kill "$p"; done
sleep 1
am force-stop com.huskerminion.showassist >/dev/null 2>&1
if [ "$1" = "nohal" ]; then
  setprop ctl.stop vendor.audio-hal
  sleep 2
  echo "vendor.audio-hal: $(getprop init.svc.vendor.audio-hal)"
fi
echo "--- holders before start:"
sh $D/holders.sh
mkdir -p /data/misc/techo5 /data/techo5
chmod 755 $D/echod
logcat -c
cd $D && (nohup ./echod run > $D/echod.log 2>&1 &)
sleep 1
echo "echod pid: $(pidof echod)"
