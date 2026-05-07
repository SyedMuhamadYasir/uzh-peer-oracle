#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ -z "${UZH_PEER_ORACLE_TOKEN:-}" ]]; then
  echo "error: UZH_PEER_ORACLE_TOKEN is not set" >&2
  echo "export UZH_PEER_ORACLE_TOKEN=\"change-me-long-random\"" >&2
  exit 1
fi

echo "# step 1/4: doctor"
./scripts/contabo_doctor.sh

echo "# step 2/4: measure before"
./scripts/contabo_measure_before.sh

echo "# step 3/4: agent once"
./scripts/contabo_agent_once.sh

echo "# step 4/4: measure after"
./scripts/contabo_measure_after.sh

cat <<'EOF'

Next steps:
1. Inspect ./data/contabo_peer_perf_*.jsonl for peer-count and peer-stability changes.
2. Verify the oracle saw the node:
   curl -H "Authorization: Bearer $UZH_PEER_ORACLE_TOKEN" http://127.0.0.1:8787/v1/debug/nodes
3. If the one-shot run looks good, start the continuous helper:
   ./scripts/contabo_agent_loop.sh
4. If peering still looks weak, review:
   ./scripts/contabo_doctor.sh
   ./bin/uzh-peer-oracle explain-peer-failure --config configs/uzhethpow.contabo.oracle.yml --node-id <source> --target-node-id <target>
EOF
