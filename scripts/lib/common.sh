# shellcheck shell=bash
# Shared helpers for Cairn build + installer scripts.
# Source this file; do not execute it directly.

# Log an info line to stderr.
cairn::log() {
  printf '[cairn] %s\n' "$*" >&2
}

# Print an error and exit non-zero.
cairn::die() {
  printf '[cairn] error: %s\n' "$*" >&2
  exit 1
}

# Exit unless running as root.
cairn::require_root() {
  if [[ ${EUID} -ne 0 ]]; then
    cairn::die "must run as root"
  fi
}

# Prompt for a password twice (hidden input) and confirm they match.
# Usage: cairn::prompt_password_confirmed "User" out_var_name
cairn::prompt_password_confirmed() {
  local label="$1"
  local out_var="$2"
  local pass1 pass2

  while true; do
    read -rsp "${label} password: " pass1
    echo
    read -rsp "Confirm ${label} password: " pass2
    echo
    if [[ -z "$pass1" ]]; then
      echo "Password cannot be empty. Try again."
      continue
    fi
    if [[ "$pass1" != "$pass2" ]]; then
      echo "Passwords do not match. Try again."
      continue
    fi
    break
  done

  printf -v "$out_var" '%s' "$pass1"
}
