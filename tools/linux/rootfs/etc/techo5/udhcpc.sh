#!/bin/sh
# udhcpc hook for the TECHO5 root filesystem. The root is read-only, so the
# resolver file lives on /run (/etc/resolv.conf is a symlink to it); busybox's
# stock script would try to write a temporary file next to it.
RESOLV=/run/resolv.conf

case "$1" in
deconfig)
	ip link set "$interface" up
	ip -4 addr flush dev "$interface"
	;;
bound|renew)
	ifconfig "$interface" "$ip" netmask "${subnet:-255.255.255.0}" ${broadcast:+broadcast $broadcast}
	if [ -n "$router" ]; then
		while ip route del default dev "$interface" 2>/dev/null; do :; done
		for r in $router; do
			ip route add default via "$r" dev "$interface" && break
		done
	fi
	{
		[ -n "$domain" ] && echo "search $domain"
		for d in $dns; do echo "nameserver $d"; done
	} > "$RESOLV.tmp" && mv -f "$RESOLV.tmp" "$RESOLV"
	;;
esac
exit 0
