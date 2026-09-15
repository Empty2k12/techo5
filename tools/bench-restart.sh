#!/system/bin/sh
# Bench helper: stop everything that holds the audio devices, then start the daemon fresh.
# Usage (as root): sh /data/local/tmp/bench-restart.sh [android]
#
# Android's audioserver is stopped, and stays stopped, while the daemon runs. Measured
# 2026-09-14: if audioserver runs while the daemon holds the PCM devices, the vendor HAL fails to
# open them, audioserver crash-loops once a second, system_server dies with it, and the framework
# restart hangs until the daemon lets go. With audioserver stopped the framework stays up and only
# AudioService logs "Audioserver died" twice a second. `setprop ctl.start audioserver` brings
# Android audio back once the daemon is gone.
#   android - restore Android audio (start audioserver) instead of starting the daemon
D=/data/local/tmp
for p in $(pidof echod); do kill "$p"; done
sleep 1
am force-stop com.huskerminion.showassist >/dev/null 2>&1
if [ "$1" = "android" ]; then
  setprop ctl.start audioserver
  sleep 2
  echo "audioserver: $(getprop init.svc.audioserver)"
  am start -n com.huskerminion.showassist/com.msp1974.vacompanion.MainActivity >/dev/null 2>&1
  exit 0
fi
setprop ctl.stop audioserver
sleep 2
echo "audioserver: $(getprop init.svc.audioserver)"
echo "--- holders before start:"
sh $D/holders.sh
mkdir -p /data/misc/techo5 /data/techo5
chmod 755 $D/echod
logcat -c
cd $D && (nohup ./echod run > $D/echod.log 2>&1 &)
sleep 1
echo "echod pid: $(pidof echod)"
