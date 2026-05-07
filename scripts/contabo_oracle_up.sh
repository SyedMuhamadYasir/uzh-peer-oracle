#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ -z "${UZH_PEER_ORACLE_TOKEN:-}" ]]; then
  echo "error: UZH_PEER_ORACLE_TOKEN is not set" >&2
  echo "export UZH_PEER_ORACLE_TOKEN=\"change-me-long-random\"" >&2
  exit 1
fi

echo "# starting Contabo oracle on :8787"
exec ./bin/uzh-peer-oracle server \
  --config configs/uzhethpow.contabo.oracle.yml \
  --seed configs/uzhethpow.contabo.peers.txt
