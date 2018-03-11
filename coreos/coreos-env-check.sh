#!/bin/bash

set -e

WARNINGS=0

echo 'Checking VirtualBox host-only networks...'
echo 'Checking /etc/exports...'
echo 'Checking /etc/hosts...'

readonly ETC_HOSTS_COREOS="$( grep 'coreos1' '/etc/hosts' )"
readonly ETC_EXPORTS_USERS="$( grep '/Users1' '/etc/exports' )"
VBoxManage list hostonlyifs

if [[ -z "${ETC_EXPORTS_USERS}" ]]
then
  WARNINGS=$((WARNINGS + 1))
  echo -en '\033[0;33m'
  echo 'There is no "/Users" record in your /etc/exports file, consider adding a line in form of:'
  echo '/Users -network 192.168.56.0 -mask 255.255.255.0 -alldirs -maproot=root:wheel'
  echo -en '\033[0m'
fi

if [[ -z "${ETC_HOSTS_COREOS}" ]]
then
  WARNINGS=$((WARNINGS + 1))
  echo -en '\033[0;33m'
  echo 'There is no "coreos" record in your /etc/hosts file, consider adding a line in form of:'
  echo '192.168.56.250	coreos.mymachine	coreos'
  echo -en '\033[0m'
fi

if [[ ${WARNINGS} -ne 0 ]]
then
  read -p 'Continue with CoreOS setup? [y/N]: ' -r shouldContinue
  if [[ "${shouldContinue}" == 'n' || "${shouldContinue}" == 'N' ]]
  then
    exit 1
  fi
fi
