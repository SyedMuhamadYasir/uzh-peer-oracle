#!/usr/bin/env bash
set -euo pipefail

DURATION_SECONDS="${1:-120}"
INTERVAL_SECONDS="${2:-5}"

GETH_BIN="/home/ethereum/uzhethereum/geth"
IPC_PATH="/home/ethereum/uzhethereum/blockchain/geth.ipc"
OUT_DIR="./data"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="${OUT_DIR}/contabo_peer_perf_${STAMP}.jsonl"

mkdir -p "${OUT_DIR}"

attach_exec() {
  local expr="$1"
  "${GETH_BIN}" attach --exec "${expr}" "${IPC_PATH}"
}

snapshot_expr='
var peerCount = net.peerCount;
var peers = admin.peers || [];
var blockNumber = eth.blockNumber;
var syncing = eth.syncing;
var row = {
  timestamp: (new Date()).toISOString(),
  net_peerCount: peerCount,
  admin_peers_length: peers.length,
  eth_blockNumber: blockNumber,
  eth_syncing: syncing,
  peers: peers.map(function(p) {
    return {
      id: p.id,
      name: p.name,
      remoteAddress: p.network ? p.network.remoteAddress : "",
      inbound: p.network ? p.network.inbound : false,
      static: p.network ? p.network.static : false,
      trusted: p.network ? p.network.trusted : false,
      caps: p.caps || [],
      protocols_eth: p.protocols && p.protocols.eth ? p.protocols.eth : null
    };
  })
};
console.log(JSON.stringify(row));
'

echo "# writing peer performance samples to ${OUT_FILE}"
end_ts="$(( $(date +%s) + DURATION_SECONDS ))"
while [ "$(date +%s)" -lt "${end_ts}" ]; do
  attach_exec "${snapshot_expr}" | tee -a "${OUT_FILE}" >/dev/null
  sleep "${INTERVAL_SECONDS}"
done

echo "# done: ${OUT_FILE}"
