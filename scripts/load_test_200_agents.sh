#!/usr/bin/env bash
set -euo pipefail

ORACLE="${ORACLE:-http://127.0.0.1:8787}"
DURATION="${DURATION:-120s}"

uzh-peer-oracle load-test --agents 200 --duration "${DURATION}" --oracle "${ORACLE}"
