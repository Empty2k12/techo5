#!/bin/sh
# Read the breadcrumbs and boot log that tools/linux/init leaves in the spare
# area of the MISC partition. Run on the device (TWRP shell or Android as root):
#   adb shell < tools/linux/readmisc.sh
MISC=/dev/block/mmcblk0p8
[ -e $MISC ] || MISC=/dev/mmcblk0p8
echo "=== bootloader message (first 64 bytes)"
dd if=$MISC bs=64 count=1 2>/dev/null | od -c | head -4
echo "=== crumbs"
dd if=$MISC bs=512 skip=8 count=16 2>/dev/null | tr -d '\000' | grep TECHO5
echo "=== boot log"
dd if=$MISC bs=512 skip=128 count=240 2>/dev/null | tr -d '\000'
