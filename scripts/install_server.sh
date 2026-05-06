#!/usr/bin/env bash
set -euo pipefail

install -d -m 0755 /etc/uzh-peer-oracle
install -d -m 0750 /var/lib/uzh-peer-oracle
install -m 0644 configs/oracle.example.yml /etc/uzh-peer-oracle.yml
install -m 0644 configs/peers.example.txt /etc/uzh-peer-oracle/peers.txt
echo "Set UZH_PEER_ORACLE_TOKEN in /etc/uzh-peer-oracle/env before starting systemd."
