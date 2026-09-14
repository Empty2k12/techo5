#!/system/bin/sh
# Lists processes holding the capture/playback/control audio devices. Run as root.
for p in /proc/[0-9]*; do
  pid=${p#/proc/}
  comm=$(cat $p/comm 2>/dev/null)
  ls -l $p/fd 2>/dev/null | grep -E 'pcmC0D2[23]|controlC0' | sed "s|^.*-> |$comm ($pid) -> |"
done
