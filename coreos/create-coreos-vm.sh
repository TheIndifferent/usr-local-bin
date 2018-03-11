#!/bin/bash

set -e

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
while [ -h "$SCRIPT_SOURCE" ]; do # resolve $SOURCE until the file is no longer a symlink
  SCRIPT_DIR="$( cd -P "$( dirname "$SCRIPT_SOURCE" )" && pwd )"
  SOURCE="$(readlink "$SOURCE")"
  [[ $SCRIPT_SOURCE != /* ]] && SOURCE="$SCRIPT_DIR/$SCRIPT_SOURCE" # if $SOURCE was a relative symlink, we need to resolve it relative to the path where the symlink file was located
done
SCRIPT_DIR="$( cd -P "$( dirname "$SCRIPT_SOURCE" )" && pwd )"

#source "${SCRIPT_DIR}/coreos-env-check.sh"

readonly COREOS_DOWNLOAD_SITE='https://stable.release.core-os.net/amd64-usr/'
readonly COREOS_CLOUD_CONFIG_GENERATOR="$SCRIPT_DIR/create-cloud-config-coreos.sh"

LATEST_VERSION="$( curl 'https://stable.release.core-os.net/amd64-usr/current/version.txt' 2>/dev/null | grep 'COREOS_VERSION_ID' | sed 's/^.*=//g' )"

read -p "Downloads dir [${HOME}/Downloads]: " -r downloads
[[ -z "${downloads}" ]] && downloads="${HOME}/Downloads"

read -p "CoreOS version [${LATEST_VERSION}]: " -r coreosversion
[[ -z "${coreosversion}" ]] && coreosversion="${LATEST_VERSION}"

read -p 'VM name [CoreOS]: ' -r vmname
[[ -z "${vmname}" ]] && vmname='CoreOS'

read -p 'CPUs [2]: ' -r cpus
[[ -z "${cpus}" ]] && cpus='2'

read -p 'RAM [8192]: ' -r ram
[[ -z "${ram}" ]] && ram='8192'

read -p 'HDD [49152]: ' -r hdd
[[ -z "${hdd}" ]] && hdd='49152'

readonly DOWNLOAD_DIR="${downloads}"
readonly VERSION="${coreosversion}"
readonly VMNAME="${vmname}"
readonly IMAGE="coreos_production_image-${VERSION}"
readonly IMAGEBIN="${IMAGE}.bin"
readonly IMAGEBZ2="${IMAGEBIN}.bz2"
readonly IMAGEBZ2DIGESTS="${IMAGEBZ2}.DIGESTS"
readonly IMAGEVDI="${VMNAME}_${IMAGE}.vdi"
readonly CONFIGDRIVE="${VMNAME}_configdrive.iso"

if ( VBoxManage list vms | grep -q "${VMNAME}" )
then
  echo "VM called '${VMNAME}' already exists" >&2
  exit 1
fi

##
## download or find existing image:
##
(
  cd "${DOWNLOAD_DIR}"
  if [[ -s "${IMAGEBZ2DIGESTS}" ]]
  then
    echo "existing checksum file found: '${DOWNLOAD_DIR}/${IMAGEBZ2DIGESTS}'"
  else
    wget -O "${IMAGEBZ2DIGESTS}" "${COREOS_DOWNLOAD_SITE}${VERSION}/coreos_production_image.bin.bz2.DIGESTS"
  fi
  if [[ -s "${IMAGEBZ2}" ]]
  then
    echo "existing image file found: '${DOWNLOAD_DIR}/${IMAGEBZ2}'"
  else
    wget -O "${IMAGEBZ2}" "${COREOS_DOWNLOAD_SITE}${VERSION}/coreos_production_image.bin.bz2"
  fi
  echo "checksum verification..."
  if grep -q "$( md5 -q "${IMAGEBZ2}" )" "${IMAGEBZ2DIGESTS}"
  then
    echo "checksum matched"
  else
    echo "checksum verification failed for image: '${DOWNLOAD_DIR}/${IMAGEBZ2}'"
    exit 1
  fi
)

##
## creating the VM:
##
echo 'creating the VM...'
VBoxManage createvm --name "${VMNAME}" --ostype 'Linux_64' --register
VBoxManage modifyvm "${VMNAME}" --memory "${ram}" --cpus "${cpus}" --defaultfrontend headless \
           --nic1 nat --nic2 hostonly --natdnshostresolver1 on --hostonlyadapter2 vboxnet0 \
           --usb off --audio none

readonly VMCFG="$( VBoxManage showvminfo "${VMNAME}" --machinereadable | grep 'CfgFile' | sed -e 's/CfgFile="//g' -e 's/"//g' )"
readonly VMDIR="$( dirname "$VMCFG" )"

##
## moving to VM dir:
##
cd "${VMDIR}"

##
## creating the cloud config ISO:
##
echo 'generating cloud config...'
mkdir -p "${VMDIR}/cloud_config/openstack/latest"
bash "$COREOS_CLOUD_CONFIG_GENERATOR" "${VMDIR}/${CONFIGDRIVE}"
if [[ $? -ne 0 ]]
then
  echo 'Config generation failed' >&2
  exit 1
fi

##
## creating the main HDD from CoreOS image:
##
echo 'unpacking the image...'
bzip2 -d -k -c "${DOWNLOAD_DIR}/${IMAGEBZ2}" >"${IMAGEBIN}"
echo 'converting the image...'
VBoxManage convertfromraw "${IMAGEBIN}" "${IMAGEVDI}" --format VDI
if [[ -s "${IMAGEBIN}" ]]
then
  rm -f "${IMAGEBIN}"
fi
#VBoxManage clonehd "IMAGEVDI" coreos.vdi
#VBoxManage modifymedium coreos.vdi --resize 49152
echo 'resizing the image...'
VBoxManage modifymedium "${IMAGEVDI}" --resize "${hdd}"

echo 'adding drives to the VM...'
#VBoxManage modifymedium "$IMAGEVDI" --move "$VMDIR"
VBoxManage storagectl "${VMNAME}" --name 'IDE' --add ide --controller PIIX4 --portcount 2 --bootable on --hostiocache on
VBoxManage storageattach "${VMNAME}" --storagectl 'IDE' --port 0 --device 0 --type hdd --medium "${VMDIR}/${IMAGEVDI}" --nonrotational on --discard on
VBoxManage storageattach "${VMNAME}" --storagectl 'IDE' --port 1 --device 0 --type dvddrive --medium "${VMDIR}/${CONFIGDRIVE}"

##
## finished
##
echo "${VMNAME} created."
