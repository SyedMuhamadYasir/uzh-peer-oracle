#!/usr/bin/env bash
set -euo pipefail

ORACLE="${ORACLE:-http://127.0.0.1:8787}"
TOKEN="${UZH_PEER_ORACLE_TOKEN:?set UZH_PEER_ORACLE_TOKEN}"

for _ in $(seq 1 12); do
  curl -fsS -H "Authorization: Bearer ${TOKEN}" "${ORACLE}/v1/debug/nodes" | jq '[.[] | {name:.node_name, peers:.peer_count, zones:.zones}]'
  sleep 10
done
