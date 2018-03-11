#!/bin/bash

exec 1>>'/var/log/openvpn-up.txt'
exec 2>>'/var/log/openvpn-up.txt'

set -x

## forbidding all possible network on drop:
/usr/sbin/iptables -P INPUT DROP
/usr/sbin/iptables -P OUTPUT DROP
/usr/sbin/iptables -P FORWARD DROP

cat /root/resolv.conf >/etc/resolv.conf
