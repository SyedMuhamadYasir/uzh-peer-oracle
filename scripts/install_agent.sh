#!/usr/bin/env bash
set -euo pipefail

install -d -m 0750 /var/lib/uzh-peer-agent
install -m 0644 configs/agent.example.yml /etc/uzh-peer-agent.yml
echo "Edit /etc/uzh-peer-agent.yml, set UZH_PEER_ORACLE_TOKEN, then run diagnose."
