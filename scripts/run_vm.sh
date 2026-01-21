#!/bin/bash

set -e

SCRIPT=$(realpath -s "${0}")
SCRIPTS_PATH=$(dirname "${SCRIPT}")
TESTS_PATH=$(realpath -s "${SCRIPTS_PATH}/../tests")

: "${ELMNTL_PREFIX:=}" 
: "${ELMNTL_FIRMWARE:=/usr/share/qemu/ovmf-x86_64.bin}"
: "${ELMNTL_FWDIP:=127.0.0.1}"
: "${ELMNTL_FWDPORT:=2222}"
: "${ELMNTL_MEMORY:=4096}"
: "${ELMNTL_LOGFILE:=${TESTS_PATH}/${ELMNTL_PREFIX}serial.log}"
: "${ELMNTL_PIDFILE:=${TESTS_PATH}/${ELMNTL_PREFIX}testvm.pid}"
: "${ELMNTL_TESTDISK:=${TESTS_PATH}/${ELMNTL_PREFIX}testdisk.qcow2}"
: "${ELMNTL_VMSTDOUT:=${TESTS_PATH}/${ELMNTL_PREFIX}vmstdout}"
: "${ELMNTL_DISKSIZE:=16G}"
: "${ELMNTL_DISPLAY:=none}"
: "${ELMNTL_ACCEL:=kvm}"
: "${ELMNTL_TARGETARCH:=$(uname -p)}"
: "${ELMNTL_MACHINETYPE:=q35}"
: "${ELMNTL_CPU:=max}"
: "${ELMNTL_SMP:=}"
: "${ELMNTL_DEBUG:=false}"
: "${ELMNTL_BRIDGE:=}"
: "${ELMNTL_MAC:=52:54:00:12:34:56}"

function _abort {
  echo "$@" && exit 1
}

function start {
  local base_disk=$1
  local usrnet_arg="-netdev user,id=user0,hostfwd=tcp:${ELMNTL_FWDIP}:${ELMNTL_FWDPORT}-:22 -device virtio-net-pci,romfile=,netdev=user0,mac=${ELMNTL_MAC}"
  local brnet_arg="-device virtio-net-pci,netdev=user0,mac=${ELMNTL_MAC} -netdev bridge,id=user0,br=${ELMNTL_BRIDGE}"
  local accel_arg
  local memory_arg="-m ${ELMNTL_MEMORY}"
  local firmware_arg="-drive if=pflash,format=raw,unit=0,readonly=on,file=${ELMNTL_FIRMWARE}"
  local disk_arg="-drive file=${ELMNTL_TESTDISK},if=none,id=disk,format=qcow2,media=disk -device virtio-blk-pci,drive=disk,bootindex=1"
  local serial_arg="-serial file:${ELMNTL_LOGFILE}"
  local pidfile_arg="-pidfile ${ELMNTL_PIDFILE}"
  local rngdev_arg="-device virtio-rng-pci"
  local display_arg="-display ${ELMNTL_DISPLAY}"
  local machine_arg="-machine type=${ELMNTL_MACHINETYPE}"
  local cdrom_arg
  local cpu_arg
  local smp_arg
  local vmpid
  local kvm_arg
  local out

  if [[ -f "${ELMNTL_PIDFILE}" ]]; then
    vmpid=$(<${ELMNTL_PIDFILE})
    if ps -p ${vmpid} > /dev/null; then
      echo "test VM is already running with pid ${vmpid}"
      exit 0
    else
      echo "removing outdated pidfile ${ELMNTL_PIDFILE}"
      rm -f "${ELMNTL_PIDFILE}"
    fi
  fi

  [[ -f "${base_disk}" ]] || _abort "Disk not found: ${base_disk}"

  # Generate the VM disk
  case "${base_disk}" in
    *.raw)
      qemu-img convert -O qcow2 "${base_disk}" "${ELMNTL_TESTDISK}" > /dev/null
      ;;
    *.qcow2)
      qemu-img create -f qcow2 -b "${base_disk}" -F qcow2 "${ELMNTL_TESTDISK}" > /dev/null
      ;;
    *.iso)
      qemu-img create -f qcow2 "${ELMNTL_TESTDISK}" "${ELMNTL_DISKSIZE}" > /dev/null
      cdrom_arg="-drive file=${base_disk},readonly=on,if=none,id=cdrom,media=cdrom -device virtio-scsi-pci,id=scsi0 -device scsi-cd,bus=scsi0.0,drive=cdrom,bootindex=2"
      ;;
    *)
      _abort "Expected a *.qcow2 or *.iso file"
      ;;
  esac

  if [[ "${ELMNTL_ACCEL}" == "hvf" ]]; then
    accel_arg="-accel ${ELMNTL_ACCEL}"
    firmware_arg="-bios ${ELMNTL_FIRMWARE} ${firmware_arg}"
    cpu_arg="-cpu max,-pdpe1gb"
  fi

  if [[ "${ELMNTL_ACCEL}" == "kvm" ]]; then
    cpu_arg="-cpu host"
    kvm_arg="-enable-kvm"
  fi

  if [[ "${ELMNTL_DEBUG}" == "true" ]]; then
    serial_arg="-serial stdio"
  else
    out="> ${ELMNTL_VMSTDOUT} 2>&1 &"
  fi

  local net_arg="${usrnet_arg}"
  if [[ "${ELMNTL_BRIDGE}" != "" ]]; then
    net_arg="${brnet_arg}"
  fi

  [[ "${ELMNTL_SMP}" != "" ]] && smp_arg="-smp ${ELMNTL_SMP}"

  # Generate the command line
  cmdline="qemu-system-${ELMNTL_TARGETARCH} ${kvm_arg} ${disk_arg} ${cdrom_arg} ${global_arg} ${firmware_arg} \
             ${net_arg} ${memory_arg} ${graphics_arg} ${serial_arg} ${pidfile_arg} \
             ${display_arg} ${machine_arg} ${accel_arg} ${rngdev_arg} ${cpu_arg} ${smp_arg}"

  # Start the VM
  eval ${cmdline} ${out}
}

function stop {
  local vmpid
  local killprog

  if [[ -f "${ELMNTL_PIDFILE}" ]]; then
    vmpid=$(<"${ELMNTL_PIDFILE}")
    killprog=$(which kill)
    if ${killprog} --version | grep -q util-linux; then
        ${killprog} --verbose --timeout 1000 TERM --timeout 5000 KILL --signal QUIT ${vmpid}
    else
        ${killprog} -9 ${vmpid}
    fi
    rm -f "${ELMNTL_PIDFILE}"
  else
    echo "No pidfile ${ELMNTL_PIDFILE} found, nothing to stop"
  fi
}

function clean {
  ([[ -f "${ELMNTL_LOGFILE}" ]] && rm -f "${ELMNTL_LOGFILE}") || true
  ([[ -f "${ELMNTL_TESTDISK}" ]] && rm -f "${ELMNTL_TESTDISK}") || true
  ([[ -f "${ELMNTL_VMSTDOUT}" ]] && rm -f "${ELMNTL_VMSTDOUT}") || true
}

function vmpid {
    local timeout=10

    until [[ -f "${ELMNTL_PIDFILE}" ]] || ((timeout-- <= 0)); do
      sleep 1
    done

    [[ -f "${ELMNTL_PIDFILE}" ]] && cat "${ELMNTL_PIDFILE}"
}

cmd=$1
disk=$2

case $cmd in
  start)
    start "${disk}"
    ;;
  stop)
    stop
    ;;
  clean)
    clean
    ;;
  vmpid)
    vmpid
    ;;
  *)
    _abort "Unknown command: ${cmd}"
    ;;
esac

exit 0
