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