#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "# measuring current peer state for 60 seconds"
exec ./scripts/contabo_peer_perf.sh 60 5
