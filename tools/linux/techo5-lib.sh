# techo5-lib.sh — shell functions shared by the initramfs init (tools/linux/init)
# and the root filesystem's boot script (tools/linux/rootfs/etc/techo5/boot.sh).
# Busybox sh. The caller may define log() before sourcing; the default goes to
# the kernel log.

type log >/dev/null 2>&1 || log() { echo "techo5: $*" > /dev/kmsg; }

# t5_usb_acm: one CDC ACM serial function on the USB gadget (a COM port on the
# host). Idempotent. The 4.9.77 (TWRP) kernel has the legacy android_usb
# gadget, the 4.9.337 (LineageOS) kernel has configfs; both are handled.
t5_usb_acm() {
	A=/sys/class/android_usb/android0
	G=/sys/kernel/config/usb_gadget/g1
	if [ -d $A ]; then
		[ "$(cat $A/functions 2>/dev/null)" = acm ] && [ "$(cat $A/enable 2>/dev/null)" = 1 ] && return 0
		echo 0 > $A/enable
		echo 1d6b > $A/idVendor
		echo 0104 > $A/idProduct
		echo TECHO5 > $A/iManufacturer
		echo "Echo Show 5 Linux" > $A/iProduct
		echo techo5 > $A/iSerial
		echo acm > $A/functions
		[ -e $A/f_acm/instances ] && echo 1 > $A/f_acm/instances
		echo 1 > $A/enable && log "usb: legacy gadget enabled (acm)" || log "usb: legacy gadget enable failed"
		return 0
	fi
	mountpoint -q /sys/kernel/config || mount -t configfs none /sys/kernel/config 2>/dev/null || { log "usb: configfs mount failed"; return 1; }
	if [ -d $G ] && [ -n "$(cat $G/UDC 2>/dev/null)" ]; then
		return 0
	fi
	mkdir -p $G || return 1
	echo 0x1d6b > $G/idVendor
	echo 0x0104 > $G/idProduct
	echo 0x0200 > $G/bcdUSB
	mkdir -p $G/strings/0x409
	echo "TECHO5" > $G/strings/0x409/manufacturer
	echo "Echo Show 5 Linux" > $G/strings/0x409/product
	echo "techo5" > $G/strings/0x409/serialnumber
	mkdir -p $G/configs/c.1/strings/0x409
	echo "acm" > $G/configs/c.1/strings/0x409/configuration
	mkdir -p $G/functions/acm.usb0
	[ -e $G/configs/c.1/acm.usb0 ] || ln -s $G/functions/acm.usb0 $G/configs/c.1/acm.usb0
	UDC=$(ls /sys/class/udc 2>/dev/null | head -1)
	if [ -n "$UDC" ]; then
		echo "$UDC" > $G/UDC && log "usb: gadget bound to $UDC (acm)" || log "usb: bind to $UDC failed"
	else
		log "usb: no UDC; gadget not started"
	fi
	[ -e /dev/ttyGS0 ] || mdev -s
}

# t5_wifi_conf <file>: write a wpa_supplicant configuration from the network
# Android had saved on userdata, unless <file> exists already. Nothing is typed
# and nothing leaves the device.
t5_wifi_conf() {
	out=$1
	[ -s "$out" ] && return 0
	xml=/data/misc/apexdata/com.android.wifi/WifiConfigStore.xml   # Android 11
	[ -r "$xml" ] || xml=/data/misc/wifi/WifiConfigStore.xml         # older Android
	[ -r "$xml" ] || { log "wifi: no configuration at $out and no Android store to take one from"; return 1; }
	xmlval() { sed -n "s/.*<string name=\"$1\">\(.*\)<\/string>.*/\1/p" "$xml" | head -1 | sed 's/&quot;//g; s/&amp;/\&/g; s/&lt;/</g; s/&gt;/>/g'; }
	ssid=$(xmlval SSID); psk=$(xmlval PreSharedKey)
	[ -n "$ssid" ] && [ -n "$psk" ] || { log "wifi: no PSK network in $xml"; return 1; }
	mkdir -p "$(dirname "$out")"
	umask 077
	printf 'ctrl_interface=/run/wpa\nupdate_config=0\nnetwork={\n\tssid="%s"\n\tpsk="%s"\n}\n' "$ssid" "$psk" > "$out"
	umask 022
	log "wifi: configuration written from Android's saved network '$ssid'"
}

# t5_wifi_up <module.ko> <wpa.conf>: load the vendor driver if wlan0 is not
# there yet, associate, and take a DHCP lease (udhcpc stays running to renew
# it). Sets IP. The vendor driver's first full scan alone takes several
# seconds; association is allowed WIFI_WAIT seconds (60).
t5_wifi_up() {
	mod=$1; conf=$2; IP=
	if ! ip link show wlan0 >/dev/null 2>&1; then
		[ -e "$mod" ] || { log "wifi: driver not found at $mod"; return 1; }
		insmod "$mod" 2>/tmp/insmod.err || { log "wifi: insmod failed: $(cat /tmp/insmod.err)"; return 1; }
		n=0; while [ $n -lt 10 ] && ! ip link show wlan0 >/dev/null 2>&1; do sleep 1; n=$((n+1)); done
		ip link show wlan0 >/dev/null 2>&1 || { log "wifi: driver loaded but no wlan0"; return 1; }
		log "wifi: driver loaded, wlan0 present"
	fi
	[ -r "$conf" ] || { log "wifi: no configuration"; return 1; }
	mkdir -p /run/wpa
	ip link set wlan0 up
	if ! pidof wpa_supplicant >/dev/null; then
		wpa_supplicant -B -i wlan0 -c "$conf" -P /run/wpa.pid > /tmp/wpa.log 2>&1
	fi
	n=0; while [ $n -lt ${WIFI_WAIT:-60} ]; do
		wpa_cli -p /run/wpa -i wlan0 status 2>/dev/null | grep -q '^wpa_state=COMPLETED' && break
		sleep 1; n=$((n+1))
	done
	if ! wpa_cli -p /run/wpa -i wlan0 status 2>/dev/null | grep -q '^wpa_state=COMPLETED'; then
		log "wifi: not associated ($(wpa_cli -p /run/wpa -i wlan0 status 2>/dev/null | grep wpa_state))"
		return 1
	fi
	if ! pidof udhcpc >/dev/null; then
		udhcpc -i wlan0 -b -R -t 10 -p /run/udhcpc.pid -s "${UDHCPC_SCRIPT:-/usr/share/udhcpc/default.script}" > /tmp/udhcpc.log 2>&1
	fi
	n=0; while [ $n -lt 30 ]; do
		IP=$(ip -4 addr show wlan0 2>/dev/null | sed -n 's/.*inet \([0-9.]*\).*/\1/p' | head -1)
		[ -n "$IP" ] && break
		sleep 1; n=$((n+1))
	done
	[ -n "$IP" ] || { log "wifi: associated but no lease"; return 1; }
	log "wifi: $IP on wlan0"
}

# t5_ip: the current IPv4 address on wlan0, empty if none.
t5_ip() { ip -4 addr show wlan0 2>/dev/null | sed -n 's/.*inet \([0-9.]*\).*/\1/p' | head -1; }

# t5_ntp: set the clock once from NTP (the RTC is not trusted), then write it
# to the RTC so the next boot starts closer. Bounded: an unreachable server
# must not hold the boot.
t5_ntp() {
	timeout -s KILL ${NTP_WAIT:-40} ntpd -n -q -p "${NTP_SERVER:-pool.ntp.org}" > /tmp/ntpd.log 2>&1 || { log "clock: ntp failed"; return 1; }
	hwclock -w 2>/dev/null
	log "clock: $(date)"
}

# t5_dropbear <keydir> [extra args]: SSH on port 22 with host keys kept in
# <keydir> (on userdata, so the host key is stable across images and slots).
# /etc/dropbear is a symlink to <keydir> in the rootfs; in the initramfs it is
# a directory and gets links to the keys.
t5_dropbear() {
	keydir=$1; shift
	mkdir -p "$keydir" /etc/dropbear
	for t in rsa ed25519; do
		[ -e "$keydir/dropbear_${t}_host_key" ] || dropbearkey -t $t -f "$keydir/dropbear_${t}_host_key" >/dev/null 2>&1
		[ -L /etc/dropbear ] || ln -sf "$keydir/dropbear_${t}_host_key" /etc/dropbear/dropbear_${t}_host_key
	done
	pidof dropbear >/dev/null && return 0
	dropbear -R -p 22 "$@" > /tmp/dropbear.log 2>&1 && log "ssh: dropbear listening on :22" || { log "ssh: dropbear failed to start"; return 1; }
}

# Bluetooth: the vendor driver gives a raw H4 channel (/dev/stpbt); btbridge
# turns it into hci0 through the kernel's vhci driver; BlueZ and bluez-alsa
# sit on top. Needs a kernel with CONFIG_BT + CONFIG_BT_HCIVHCI — without
# /dev/vhci this quietly does nothing, so an older boot image keeps working.
t5_bt_up() {
	mod=$1; logdir=${2:-/tmp}
	[ -e /dev/vhci ] || { log "bt: no /dev/vhci (kernel without Bluetooth); skipping"; return 1; }
	if [ ! -e /dev/stpbt ]; then
		[ -e "$mod" ] || { log "bt: driver not found at $mod"; return 1; }
		insmod "$mod" 2>/tmp/insmod-bt.err || { log "bt: insmod failed: $(cat /tmp/insmod-bt.err)"; return 1; }
		n=0; while [ $n -lt 10 ] && [ ! -e /dev/stpbt ]; do sleep 1; n=$((n+1)); done
		[ -e /dev/stpbt ] || { log "bt: driver loaded but no /dev/stpbt"; return 1; }
	fi
	command -v btbridge >/dev/null || { log "bt: no btbridge"; return 1; }
	# The factory address from IDME; the firmware otherwise comes up with a random one.
	addr=$(tr -d '\n\0' < /proc/idme/bt_mac_addr 2>/dev/null)
	(while true; do btbridge ${addr:+-bdaddr "$addr"} >> "$logdir/btbridge.log" 2>&1; sleep 2; done) &
	n=0; while [ $n -lt 10 ] && [ ! -d /sys/class/bluetooth/hci0 ]; do sleep 1; n=$((n+1)); done
	[ -d /sys/class/bluetooth/hci0 ] || { log "bt: bridge up but no hci0"; return 1; }
	# Pairings and bluez-alsa's state must survive reboots and slot changes: keep
	# them on userdata (the root is read-only).
	mkdir -p /run/dbus /data/misc/techo5/bluetooth /data/misc/techo5/bluealsa /var/lib/bluetooth /var/lib/bluealsa
	mountpoint -q /var/lib/bluetooth || mount --bind /data/misc/techo5/bluetooth /var/lib/bluetooth
	mountpoint -q /var/lib/bluealsa || mount --bind /data/misc/techo5/bluealsa /var/lib/bluealsa
	if ! pidof dbus-daemon >/dev/null; then
		dbus-daemon --system --nofork --nopidfile >> "$logdir/dbus.log" 2>&1 &
		sleep 1
	fi
	bd=$(command -v bluetoothd || echo /usr/lib/bluetooth/bluetoothd)
	[ -x "$bd" ] && "$bd" -n >> "$logdir/bluetoothd.log" 2>&1 &
	command -v bluealsa >/dev/null && bluealsa -p a2dp-source >> "$logdir/bluealsa.log" 2>&1 &
	log "bt: hci0 up; bluetoothd and bluealsa started"
	return 0
}
