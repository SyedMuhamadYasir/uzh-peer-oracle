# Operations

Status: Milestone 1 operations plus future deployment notes. The supported runnable path today is `server`, `ingest`, `diagnose`, and `agent`.

## Server

```bash
export UZH_PEER_ORACLE_TOKEN="long-random-token"

uzh-peer-oracle server \
  --config /etc/uzh-peer-oracle.yml \
  --seed /etc/uzh-peer-oracle/peers.txt
```

Put TLS in front of the oracle with Caddy, nginx, HAProxy, or an institutional load balancer.

## Agent

```bash
export UZH_PEER_ORACLE_TOKEN="long-random-token"

uzh-peer-oracle diagnose \
  --config /etc/uzh-peer-agent.yml

uzh-peer-oracle agent \
  --config /etc/uzh-peer-agent.yml
```

The agent must run as a user that can access the Geth IPC socket.

## Future Probe

Probe agents are planned for Milestone 2. The reserved command shape is:

```bash
uzh-peer-oracle probe --config /etc/uzh-peer-probe.yml
```

Recommended probes:

- one public probe near the oracle;
- one UZH VPN probe;
- one campus probe if campus and VPN have different routing;
- one lab probe per isolated lab zone.

## SQLite

SQLite WAL is enabled at startup. Keep the database on local disk, not a network filesystem.

## Backup

Back up:

- `/var/lib/uzh-peer-oracle/oracle.db*`
- `/var/lib/uzh-peer-oracle/snapshot_ed25519.key`
- `/etc/uzh-peer-oracle.yml`
- `/etc/uzh-peer-oracle/peers.txt`

Protect the snapshot private key. It signs registry views.
