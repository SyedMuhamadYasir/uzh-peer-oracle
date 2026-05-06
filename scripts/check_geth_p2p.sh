#!/usr/bin/env bash
set -euo pipefail

IPC_PATH="${IPC_PATH:-$HOME/uzhethereum/.uzhethereum/geth.ipc}"
PORT="${PORT:-30308}"
GETH_ATTACH="${GETH_ATTACH:-geth attach}"

echo "Checking TCP/UDP listeners on ${PORT}"
ss -lntup | grep ":${PORT}" || true

echo
echo "Checking IPC path: ${IPC_PATH}"
if [[ ! -S "${IPC_PATH}" ]]; then
  echo "ERROR: IPC socket not found. Check datadir and permissions."
  exit 1
fi

echo
echo "Geth nodeInfo"
${GETH_ATTACH} "${IPC_PATH}" --exec 'admin.nodeInfo'

echo
echo "Own enode"
ENODE="$(${GETH_ATTACH} "${IPC_PATH}" --exec 'admin.nodeInfo.enode')"
echo "${ENODE}"

if echo "${ENODE}" | grep -Eq '@(127\.|10\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|192\.168\.)'; then
  echo
  echo "WARNING: advertised enode IP looks private/localhost."
  echo "For public nodes, restart Geth with: --nat extip:PUBLIC_IP"
fi

echo
echo "Own ENR"
${GETH_ATTACH} "${IPC_PATH}" --exec 'admin.nodeInfo.enr' || true
