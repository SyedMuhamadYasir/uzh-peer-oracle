#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "# running Contabo doctor"
exec ./bin/uzh-peer-oracle contabo-doctor --config configs/uzhethpow.contabo.agent.yml
