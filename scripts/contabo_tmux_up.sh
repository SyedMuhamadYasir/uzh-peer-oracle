#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

SESSION="uzh-peer-oracle"

if ! command -v tmux >/dev/null 2>&1; then
  echo "error: tmux is not installed" >&2
  exit 1
fi

if tmux has-session -t "${SESSION}" 2>/dev/null; then
  echo "# tmux session ${SESSION} already exists"
  exit 0
fi

tmux new-session -d -s "${SESSION}" -n oracle
tmux send-keys -t "${SESSION}:oracle" "cd \"${ROOT_DIR}\"" C-m
tmux send-keys -t "${SESSION}:oracle" "echo 'export UZH_PEER_ORACLE_TOKEN=\"change-me-long-random\"'" C-m
tmux send-keys -t "${SESSION}:oracle" "echo './scripts/contabo_oracle_up.sh'" C-m

tmux new-window -t "${SESSION}" -n ops
tmux send-keys -t "${SESSION}:ops.0" "cd \"${ROOT_DIR}\"" C-m
tmux send-keys -t "${SESSION}:ops.0" "echo './scripts/contabo_doctor.sh'" C-m
tmux split-window -h -t "${SESSION}:ops"
tmux send-keys -t "${SESSION}:ops.1" "cd \"${ROOT_DIR}\"" C-m
tmux send-keys -t "${SESSION}:ops.1" "echo 'curl -H \"Authorization: Bearer \$UZH_PEER_ORACLE_TOKEN\" http://127.0.0.1:8787/v1/debug/nodes'" C-m

tmux new-window -t "${SESSION}" -n test
tmux send-keys -t "${SESSION}:test.0" "cd \"${ROOT_DIR}\"" C-m
tmux send-keys -t "${SESSION}:test.0" "echo './scripts/contabo_measure_before.sh'" C-m
tmux split-window -h -t "${SESSION}:test"
tmux send-keys -t "${SESSION}:test.1" "cd \"${ROOT_DIR}\"" C-m
tmux send-keys -t "${SESSION}:test.1" "echo './scripts/contabo_agent_once.sh'" C-m

echo "# created tmux session ${SESSION}"
echo "# attach with: ./scripts/contabo_tmux_attach.sh"
