#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../scripts/lib/common.sh"
  source "${BATS_TEST_DIRNAME}/../scripts/lib/target.sh"
  TMP="$(mktemp -d)"
  SRC="${TMP}/skel"
  DEST="${TMP}/target/etc/skel"

  mkdir -p "${SRC}/.config/hypr/scripts"
  echo 'source = ./colors.conf' > "${SRC}/.config/hypr/hyprland.conf"
  printf '#!/usr/bin/env bash\n' > "${SRC}/.config/hypr/scripts/screenshot.sh"
  chmod 755 "${SRC}/.config/hypr/scripts/screenshot.sh"
}

teardown() {
  rm -rf "${TMP}"
}

@test "cairn::install_skel copies dotfiles into the target skeleton" {
  run cairn::install_skel "${SRC}" "${DEST}"
  [ "$status" -eq 0 ]
  [ -f "${DEST}/.config/hypr/hyprland.conf" ]
  [[ "$(cat "${DEST}/.config/hypr/hyprland.conf")" == 'source = ./colors.conf' ]]
}

@test "cairn::install_skel preserves the exec bit on helper scripts" {
  cairn::install_skel "${SRC}" "${DEST}"
  [ -x "${DEST}/.config/hypr/scripts/screenshot.sh" ]
}

@test "cairn::install_skel merges into an existing skeleton without removing it" {
  mkdir -p "${DEST}"
  echo 'stock' > "${DEST}/.bash_profile"
  cairn::install_skel "${SRC}" "${DEST}"
  [ -f "${DEST}/.bash_profile" ]
  [ -f "${DEST}/.config/hypr/hyprland.conf" ]
}

@test "cairn::install_skel is a no-op when the source is missing" {
  run cairn::install_skel "${TMP}/absent" "${DEST}"
  [ "$status" -eq 0 ]
  [ ! -e "${DEST}" ]
  [[ "$output" == *"default /etc/skel"* ]]
}
