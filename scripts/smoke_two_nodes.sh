#!/usr/bin/env bash
set -euo pipefail

ORACLE="${ORACLE:-http://127.0.0.1:8787}"
TOKEN="${UZH_PEER_ORACLE_TOKEN:?set UZH_PEER_ORACLE_TOKEN}"

curl -fsS -H "Authorization: Bearer ${TOKEN}" "${ORACLE}/v1/debug/nodes" | jq 'length'
echo "Start two agents, then watch net.peerCount rise without static-nodes.json."
