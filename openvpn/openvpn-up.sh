#!/bin/bash

exec 1>>'/var/log/openvpn-up.txt'
exec 2>>'/var/log/openvpn-up.txt'

set -x

echo 'env:'
env

## setting default routes:
/usr/sbin/ip route del default
/usr/sbin/ip route add default via "$route_vpn_gateway"

## updating resolv.conf:
echo "nameserver $( echo "$foreign_option_1" | awk '{ print $3 }' )" >'/etc/resolv.conf'
echo 'nameserver 8.8.8.8' >>'/etc/resolv.conf'

## clearing iptables rules:
/usr/sbin/iptables -F
/usr/sbin/iptables -X
/usr/sbin/iptables -t nat -F
/usr/sbin/iptables -t nat -X
/usr/sbin/iptables -t mangle -F
/usr/sbin/iptables -t mangle -X

## allowing all localhost operations:
/usr/sbin/iptables -A INPUT -i lo -j ACCEPT
/usr/sbin/iptables -A OUTPUT -o lo -j ACCEPT

## allowing lan:
/usr/sbin/iptables -A INPUT -s 192.168.1.0/24 -d 192.168.1.0/24 -j ACCEPT
/usr/sbin/iptables -A OUTPUT -s 192.168.1.0/24 -d 192.168.1.0/24 -j ACCEPT

## allowing communication between eth0 and tun0:
/usr/sbin/iptables -A FORWARD -i enp3s0 -o tun0 -j ACCEPT
/usr/sbin/iptables -A FORWARD -i tun0 -o enp3s0 -j ACCEPT

/usr/sbin/iptables -t nat -A POSTROUTING -o tun0 -j MASQUERADE

## dropping all traffic except through openvpn:
/usr/sbin/iptables -A OUTPUT -o enp3s0 ! -d "$trusted_ip" -j DROP
