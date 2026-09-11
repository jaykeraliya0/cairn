#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../scripts/lib/common.sh"
}

@test "cairn::log writes prefixed line" {
  run cairn::log "hello"
  [ "$status" -eq 0 ]
  [[ "$output" == *"[cairn] hello"* ]]
}

@test "cairn::die exits non-zero with message" {
  run cairn::die "boom"
  [ "$status" -ne 0 ]
  [[ "$output" == *"error: boom"* ]]
}
@test "cairn::read_package_list drops comments, trailing comments and blanks" {
  local list="${BATS_TEST_TMPDIR}/packages.x86_64"
  printf '# header\n\nbase\nlinux  # with a trailing comment\n\n  # indented comment\nsudo\n' > "$list"
  run cairn::read_package_list "$list"
  [ "$status" -eq 0 ]
  [ "${lines[0]}" = "base" ]
  [ "${lines[1]}" = "linux" ]
  [ "${lines[2]}" = "sudo" ]
  [ "${#lines[@]}" -eq 3 ]
}
