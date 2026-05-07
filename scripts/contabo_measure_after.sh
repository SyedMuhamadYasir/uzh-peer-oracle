#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "# measuring peer state after one-shot peering for 120 seconds"
exec ./scripts/contabo_peer_perf.sh 120 5
