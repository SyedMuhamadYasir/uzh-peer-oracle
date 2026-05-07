#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ -z "${UZH_PEER_ORACLE_TOKEN:-}" ]]; then
  echo "error: UZH_PEER_ORACLE_TOKEN is not set" >&2
  echo "export UZH_PEER_ORACLE_TOKEN=\"change-me-long-random\"" >&2
  exit 1
fi

ORACLE_URL="${ORACLE_URL:-http://127.0.0.1:8787}"

echo "# fetching curated bootnodes TOML from ${ORACLE_URL}"
curl -fsSL \
  -H "Authorization: Bearer ${UZH_PEER_ORACLE_TOKEN}" \
  "${ORACLE_URL%/}/v1/bootnodes/toml"
