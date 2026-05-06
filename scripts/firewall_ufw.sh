#!/usr/bin/env bash
set -euo pipefail

sudo ufw allow 30308/tcp
sudo ufw allow 30308/udp
