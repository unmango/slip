#!/usr/bin/env bash
#
# Runs the instrumented tests on a headless emulator, against a git remote the
# emulator can actually reach.
#
# The remote is git daemon rather than a directory: go-git speaks the git
# protocol in process, where a filesystem remote makes it exec git-upload-pack,
# and there is no git binary on an Android device. 10.0.2.2 is the emulator's
# route back to the host.
#
# Run it from the android/ directory, inside `nix develop .#android`.

set -euo pipefail

sdk="${SLIP_EMULATOR_SDK:?not set; run inside nix develop .#android}"
api="${SLIP_ANDROID_API:-$(sed -n 's/^compileSdk=//p' sdk-versions.properties)}"

# Gradle needs the build SDK, which is the one with the platform and build
# tools the app compiles against. avdmanager and the emulator need the other
# one, which is where the system image is, and they read ANDROID_HOME in
# preference to ANDROID_SDK_ROOT. An AVD created with the wrong one records the
# wrong system image path and the emulator dies at boot saying the AVD is
# broken, which is not what is broken.
build_sdk="${ANDROID_HOME:?not set; run inside nix develop .#android}"
export ANDROID_HOME="${sdk}"

avd="slip-test"
image="system-images;android-${api};google_apis;x86_64"
console_port=5570
serial="emulator-${console_port}"
daemon_port=9418

# A cold emulator on a slow machine takes a couple of minutes. Past this it is
# not slow, it is stuck.
boot_timeout=420

adb="${sdk}/platform-tools/adb"
emulator="${sdk}/emulator/emulator"
avdmanager="$(echo "${sdk}"/cmdline-tools/*/bin/avdmanager)"

# The store is read-only, so both the AVD and anything the emulator writes go
# next to the build output instead.
export ANDROID_AVD_HOME="${PWD}/build/avd"
export ANDROID_SDK_ROOT="${sdk}"

work="$(mktemp -d)"
emulator_pid=""

# stop waits for a process, then insists. Nothing here may block the run: a
# wedged emulator that ignores `emu kill` would otherwise hold the script open
# for as long as it felt like, which is the failure this script exists to
# report rather than to reproduce.
stop() {
  local pid=$1 signal

  for signal in "" TERM KILL; do
    if [ -n "${signal}" ]; then
      kill -"${signal}" "${pid}" 2>/dev/null || true
    fi

    for _ in $(seq 1 15); do
      kill -0 "${pid}" 2>/dev/null || return 0
      sleep 1
    done
  done
}

cleanup() {
  if [ -n "${emulator_pid}" ]; then
    "${adb}" -s "${serial}" emu kill >/dev/null 2>&1 || true
    stop "${emulator_pid}"
  fi

  if [ -f "${work}/daemon.pid" ]; then
    kill "$(cat "${work}/daemon.pid")" >/dev/null 2>&1 || true
  fi

  rm -rf "${work}"
}
trap cleanup EXIT

# Recreated every run. It takes a second, and an AVD carrying a stale path to a
# system image in a store path that has been collected is a confusing failure.
rm -rf "${ANDROID_AVD_HOME:?}"
mkdir -p "${ANDROID_AVD_HOME}"

echo "creating avd ${avd}"
echo no | "${avdmanager}" create avd --name "${avd}" --package "${image}" --device pixel_6 --force

# A bare notebook for the tests to publish into. --enable=receive-pack because
# the default daemon is read-only, and publishing is the whole point.
git init --bare --initial-branch=main "${work}/notebook.git" >/dev/null
git daemon \
  --base-path="${work}" \
  --export-all \
  --enable=receive-pack \
  --port="${daemon_port}" \
  --detach \
  --pid-file="${work}/daemon.pid"

echo "booting ${avd}"
"${emulator}" -avd "${avd}" \
  -port "${console_port}" \
  -no-window \
  -no-audio \
  -no-boot-anim \
  -no-snapshot \
  -gpu swiftshader_indirect \
  -wipe-data \
  >"${work}/emulator.log" 2>&1 &
emulator_pid=$!

# Deliberately not `adb wait-for-device`: that blocks forever when the emulator
# dies on startup, which is most of the ways this goes wrong. The deadline
# covers the rest of them, where the process lives on without ever booting.
boot_deadline=$(($(date +%s) + boot_timeout))

until [ "$("${adb}" -s "${serial}" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
  if ! kill -0 "${emulator_pid}" 2>/dev/null; then
    echo "the emulator exited before finishing boot:" >&2
    tail -40 "${work}/emulator.log" >&2
    exit 1
  fi

  if [ "$(date +%s)" -ge "${boot_deadline}" ]; then
    echo "the emulator did not boot within ${boot_timeout}s:" >&2
    tail -40 "${work}/emulator.log" >&2
    exit 1
  fi

  sleep 2
done

echo "booted; running the instrumented tests"

# ANDROID_SDK_ROOT is dropped rather than corrected: Gradle refuses to run when
# it and ANDROID_HOME disagree, and by now the emulator that needed it is up.
env -u ANDROID_SDK_ROOT \
  ANDROID_SERIAL="${serial}" \
  ANDROID_HOME="${build_sdk}" \
  gradle connectedDebugAndroidTest \
  --console=plain \
  -Pslip.remote="git://10.0.2.2:${daemon_port}/notebook.git"
