#!/bin/bash

set -e

CLOUD_CONFIG=
CLOUD_CONFIG_ROOT=
CONFIG_ISO=

if [[ -z "$1" ]]
then
  exec 3>&1
else
  case "$1" in
    *.iso)
      CONFIG_ISO="$1"
      CLOUD_CONFIG_ROOT="$( dirname "$CONFIG_ISO" )/cloud_config"
      CLOUD_CONFIG="$CLOUD_CONFIG_ROOT/openstack/latest/user_data"
      mkdir -p "$( dirname "$CLOUD_CONFIG" )"
    ;;
    *)
      mkdir -p "$( dirname "$1" )"
      CLOUD_CONFIG="$1"
    ;;
  esac
  exec 3>"$CLOUD_CONFIG"
fi

read -p 'hostname [coreos]: ' -r hostname
[[ -z "$hostname" ]] && hostname='coreos'

read -p 'public ssh key [~/.ssh/id_rsa.pub]: ' -r sshkeyfile
[[ -z "$sshkeyfile" ]] && sshkeyfile=~/.ssh/id_rsa.pub
sshkey="$( cat "${sshkeyfile}" )"

users=()
users+=("$( whoami )")
if [[ -s ~/.ssh/id_rsa.pub ]] ; then
  users+=("$( cat ~/.ssh/id_rsa.pub | sed -e 's/^ssh-rsa .* //' -e 's/@.*$//' )")
else
  echo 'id_rsa.pub file is required for password-less ssh authentication' >&2
  exit 1
fi
if [[ -s ~/.ssh/config ]] ; then
  users+=("$( cat ~/.ssh/config | grep '^user' | sed 's/^.* //' )")
fi
# while true
# do
#    read -p 'add user: ' -r username
#    if [[ -z "$username" ]]
#    then
#      break
#    else
#      users+=("$username")
#    fi
# done

read -p 'VM IP address [192.168.56.250]: ' -r vmip
[[ -z "$vmip" ]] && vmip='192.168.56.250'

read -p 'host IP address [192.168.56.1]: ' -r hostip
[[ -z "$hostip" ]] && hostip='192.168.56.1'

#read -p 'NFS mount: ' -r nfsdir
read -p 'NFS mount [/Users]: ' -r nfsdir
[[ -z "$nfsdir" ]] && nfsdir='/Users'

read -p 'add /etc/resolv.conf from current system? [y/N]: ' -r addResolvConf
if [[ "$addResolvConf" == 'y' || "$addResolvConf" == 'Y' ]]
then
  nameservers="$( cat /etc/resolv.conf | grep -v '^[[:space:]]*[#;]' | grep 'nameserver' | awk '{ print $2 }' | tr '\n' ' ' )"
  searchdomains=
  if grep -q 'domain' '/etc/resolv.conf'
  then
    searchdomains="$( cat /etc/resolv.conf | grep -v '^[[:space:]]*[#;]' | grep 'domain' | awk '{ $1 = ""; print $0 }' )"
  fi
  if grep -q 'search' '/etc/resolv.conf'
  then
    searchdomains="$searchdomains $( cat /etc/resolv.conf | grep -v '^[[:space:]]*[#;]' | grep 'search' | awk '{ $1 = ""; print $0 }' )"
  fi
  searchdomains="$( echo "$searchdomains" | sed -e 's/^ *//' -e 's/ *$//' )"
fi

if [[ -z "$1" ]]
then
  echo '###########################################################'
  echo ''
fi
cat <<EOF >&3
#cloud-config
hostname: "${hostname}"
ssh_authorized_keys:
  - "${sshkey}"
EOF
if [[ ${#users[@]} -gt 0 ]]
then
  echo 'users:' >&3
  # for user in "${users[@]}"
  for user in $( echo ${users[@]} | tr ' ' '\n' | sort -u | tr '\n' ' ' )
  do
    cat <<EOF >&3
  - name: "${user}"
    groups:
      - "sudo"
      - "docker"
    ssh-authorized-keys:
      - "${sshkey}"
    sudo: ['ALL=(ALL) NOPASSWD:ALL']
EOF
  done
fi

cat <<EOF >&3
coreos:
  units:
    - name: systemd-sysctl.service
      command: restart
    - name: systemd-networkd.service
      command: stop
    - name: 00-hostonly.network
      runtime: true
      content: |
        [Match]
        Name=enp0s8
        [Network]
        Address=${vmip}/24
        LinkLocalAddressing=no
        IPv6AcceptRA=no
    - name: down-interfaces.service
      command: start
      content: |
        [Service]
        Type=oneshot
        ExecStart=/usr/bin/ip link set enp0s8 down
        ExecStart=/usr/bin/ip addr flush dev enp0s8
    - name: systemd-networkd.service
      command: restart
      enable: true
EOF
if [[ -n "$nfsdir" ]]
then
  cat <<EOF >&3
    - name: rpc-statd.service
      command: start
      enable: true
    - name: $( echo "${nfsdir}" | sed 's;/;;' ).mount
      command: start
      content: |
        [Mount]
        What=${hostip}:${nfsdir}
        Where=${nfsdir}
        Type=nfs
EOF
fi
cat <<EOF >&3
    - name: docker-tcp.socket
      command: start
      enable: true
      content: |
        [Unit]
        Description=Docker Socket for the API
        [Socket]
        ListenStream=2375
        BindIPv6Only=both
        Service=docker.service
        [Install]
        WantedBy=sockets.target
write_files:
  - path: "/etc/sysctl.d/10-disable-ipv6.conf"
    permissions: "0644"
    owner: root
    content: |
      net.ipv6.conf.all.disable_ipv6=1
      net.ipv6.conf.default.disable_ipv6=1
  - path: /etc/hosts
    permissions: "0644"
    owner: "root"
    content: |
      127.0.0.1    localhost
      ${vmip}      coreos
      ${hostip}    host
EOF

if [[ "$addResolvConf" == 'y' || "$addResolvConf" == 'Y' ]]
then
  cat <<EOF >&3
  - path: "/etc/resolv.conf"
    permissions: "0644"
    owner: "root"
    content: |
$( cat '/etc/resolv.conf' | grep -v '^[[:space:]]*[#;]' | sed 's/^ */      /g' )
  - path: /etc/systemd/resolved.conf
    permissions: "0644"
    owner: "root"
    content: |
      DNS=$nameservers
      Domains=$searchdomains
EOF
fi


if [[ -n "$CONFIG_ISO" ]]
then
  if [[ -s "$CONFIG_ISO" ]]
  then
    rm "$CONFIG_ISO"
  fi
  hdiutil makehybrid -iso -joliet -default-volume-name 'config-2' -o "$CONFIG_ISO" "$CLOUD_CONFIG_ROOT"
fi
