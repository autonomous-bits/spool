#!/usr/bin/env bash
# Seeds ADMIN_STAKEHOLDER_NAME/ADMIN_STAKEHOLDER_EMAIL in the repo-root .env from the host's
# `git config user.name`/`user.email`. compose.yaml forwards these into the spoolstore
# container, and apps/store/src/persistence/migrator.ts uses them (falling back to a generic
# placeholder when unset) to seed the bootstrap stakeholder (role='system') on first boot with a
# real, known identity instead of the generic default — so you have an existing stakeholder to
# use as a starting point for creating others.
#
# Idempotent in two ways:
#   1. Re-running this script only replaces the two ADMIN_STAKEHOLDER_* lines in .env; it never
#      duplicates them or touches any other line (e.g. GITHUB_CLIENT_ID/SECRET).
#   2. If the bootstrap stakeholder already exists in the database, the migration's
#      `ON CONFLICT (id) DO NOTHING` leaves it untouched regardless of what this script writes —
#      i.e. this never overwrites an already-seeded admin user.
#
# Usage: tools/docker/seed-admin-env.sh [path-to-env-file]
# Run this before `docker compose up --build spoolstore` (from the repo root).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
env_file="${1:-$repo_root/.env}"

git_name="$(git config --get user.name || true)"
git_email="$(git config --get user.email || true)"

if [[ -z "$git_name" || -z "$git_email" ]]; then
  echo "seed-admin-env: git config user.name/user.email not set; leaving ADMIN_STAKEHOLDER_* unset (store will fall back to its built-in bootstrap defaults)." >&2
  exit 0
fi

# git config values are a single line by construction (git rejects embedded newlines), but
# reject them defensively anyway since they'd otherwise let arbitrary extra .env entries be
# injected into the file below.
if [[ "$git_name" == *$'\n'* || "$git_email" == *$'\n'* ]]; then
  echo "seed-admin-env: git config user.name/user.email contains a newline; refusing to write .env." >&2
  exit 1
fi

touch "$env_file"

tmp_file="$(mktemp "${env_file}.XXXXXX")"
trap 'rm -f "$tmp_file"' EXIT

grep -Ev '^(ADMIN_STAKEHOLDER_NAME|ADMIN_STAKEHOLDER_EMAIL)=' "$env_file" > "$tmp_file" || true

{
  cat "$tmp_file"
  printf 'ADMIN_STAKEHOLDER_NAME=%s\n' "$git_name"
  printf 'ADMIN_STAKEHOLDER_EMAIL=%s\n' "$git_email"
} > "$env_file"

echo "seed-admin-env: set ADMIN_STAKEHOLDER_NAME/ADMIN_STAKEHOLDER_EMAIL in $env_file from git config."
