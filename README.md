# UZH Peer Oracle

Milestone 1 MVP: a working topology-aware rendezvous engine for unmodified Geth `1.10.26` nodes on the UZH teaching testnet.

This repository currently implements the core path:

```text
oracle server + SQLite WAL + seed parser + heartbeat + peer endpoint
local Geth IPC agent + admin_addPeer + managed local state
```

It does not patch Geth, does not require a Geth upgrade, does not expose Geth admin RPC publicly, and does not create or edit `static-nodes.json`.

## Target Geth

```text
Geth:        1.10.26-stable
Git commit:  e5eb32acee19cc9fca6a03b10283b7484246b15a
Network ID:  702
Chain ID:    702 / 0x2be
Consensus:   Ethash / PoW
P2P TCP:     30308
Discovery:   30308/udp
IPC path:    ~/uzhethereum/.uzhethereum/geth.ipc
```

## What Works Now

- `uzh-peer-oracle server` starts an HTTP server.
- SQLite WAL storage is enabled by default.
- Seed ingestion accepts `enode://` and `enr:` lines.
- Inline seed metadata works: `name=... zone=... role=...`.
- Bad seed lines are reported and do not crash ingestion.
- `GET /health` works without auth.
- `POST /v1/heartbeat` works with bearer-token auth.
- `GET /v1/peers` returns compatible peer recommendations.
- `GET /v1/debug/nodes` returns registered nodes with bearer-token auth.
- `uzh-peer-oracle diagnose` talks to local Geth IPC.
- `uzh-peer-oracle agent` reads local Geth IPC, heartbeats, fetches peers, calls local `admin_addPeer`, and writes managed peer state.
- Tests cover seed parsing, server API, peer selection, and fake/mock Geth IPC.

## Build

```bash
cd "/mnt/d/Download/Academic Stuff/PhD/Codex Projects/uzh-peer-oracle"
go mod tidy
go test ./...
make build
```

The binary will be written to:

```text
bin/uzh-peer-oracle
```

## Server Start

Use a long random token. For local smoke testing:

```bash
export UZH_PEER_ORACLE_TOKEN="dev-change-me"

./bin/uzh-peer-oracle server \
  --config configs/oracle.example.yml \
  --seed configs/peers.example.txt
```

Health check:

```bash
curl http://127.0.0.1:8787/health
```

Debug registered nodes:

```bash
curl -H "Authorization: Bearer $UZH_PEER_ORACLE_TOKEN" \
  http://127.0.0.1:8787/v1/debug/nodes
```

## Seed File

Example:

```text
# Public hub
enode://abc...@130.60.24.247:30308 # name=hub-public-1 zone=public role=hub

# Public ENR
enr:... # name=hub-public-2 zone=public role=hub

# UZH VPN/internal
enode://def...@10.12.3.4:30308 # name=uzh-internal-1 zone=uzh-vpn role=internal
```

Manual ingestion:

```bash
./bin/uzh-peer-oracle ingest \
  --config configs/oracle.example.yml \
  --seed configs/peers.example.txt
```

## Agent Diagnose

Edit `configs/agent.example.yml` so `geth.ipc_path` points to your real Geth IPC socket.

Then run:

```bash
export UZH_PEER_ORACLE_TOKEN="dev-change-me"

./bin/uzh-peer-oracle diagnose \
  --config configs/agent.example.yml
```

This calls local IPC methods such as:

- `admin_nodeInfo`
- `admin_peers`
- `net_version`
- `net_peerCount`
- `eth_chainId`
- `eth_blockNumber`
- `eth_getBlockByNumber`
- `web3_clientVersion`

## Agent Run

```bash
export UZH_PEER_ORACLE_TOKEN="dev-change-me"

./bin/uzh-peer-oracle agent \
  --config configs/agent.example.yml
```

The agent:

- reads local Geth identity and peer state over IPC;
- heartbeats to the oracle;
- fetches recommendations from `/v1/peers`;
- skips itself and already-connected peers;
- calls `admin_addPeer(enode)` over local IPC;
- waits briefly and checks `admin_peers`;
- records managed peers in `./data/agent-state.json`;
- never edits `static-nodes.json`;
- never requires public admin RPC.

## Exact Two-Node Smoke Test

Terminal 1, start oracle:

```bash
cd "/mnt/d/Download/Academic Stuff/PhD/Codex Projects/uzh-peer-oracle"
export UZH_PEER_ORACLE_TOKEN="dev-change-me"
./bin/uzh-peer-oracle server --config configs/oracle.example.yml
```

Terminal 2, on Geth node A:

```bash
export UZH_PEER_ORACLE_TOKEN="dev-change-me"
./bin/uzh-peer-oracle diagnose --config configs/agent.node-a.yml
./bin/uzh-peer-oracle agent --config configs/agent.node-a.yml
```

Terminal 3, on Geth node B:

```bash
export UZH_PEER_ORACLE_TOKEN="dev-change-me"
./bin/uzh-peer-oracle diagnose --config configs/agent.node-b.yml
./bin/uzh-peer-oracle agent --config configs/agent.node-b.yml
```

Check the oracle sees both:

```bash
curl -H "Authorization: Bearer $UZH_PEER_ORACLE_TOKEN" \
  http://127.0.0.1:8787/v1/debug/nodes
```

Check Geth peer counts rise on both nodes:

```bash
geth attach ~/uzhethereum/.uzhethereum/geth.ipc --exec 'net.peerCount'
geth attach ~/uzhethereum/.uzhethereum/geth.ipc --exec 'admin.peers.length'
```

Confirm no static node file was created or changed:

```bash
find ~/uzhethereum/.uzhethereum -name static-nodes.json -print
```

The expected Milestone 1 result is that two compatible public nodes discover each other via the oracle and connect through local `admin_addPeer`.

## Recommended Geth Flags

For public nodes:

```bash
PUBLIC_IP=<public ip>

./uzh-geth \
  --networkid 702 \
  --config "$HOME/uzhethereum/uzheth-config.toml" \
  --nat extip:$PUBLIC_IP \
  --port 30308 \
  --discovery.port 30308 \
  --v5disc \
  --maxpeers 100 \
  --maxpendpeers 100 \
  --http \
  --http.addr 127.0.0.1 \
  --http.port 8547 \
  --authrpc.addr 127.0.0.1 \
  --authrpc.port 8553
```

For Geth `1.10.26`, the discovery v5 flag is `--v5disc`, not `--discv5`.

## API In Milestone 1

Unauthenticated:

- `GET /health`

Authenticated:

- `POST /v1/heartbeat`
- `GET /v1/peers?node_id=...&limit=...`
- `GET /v1/debug/nodes`

The bearer token is read from `UZH_PEER_ORACLE_TOKEN` by default.

## What Remains For Milestones 2-4

Milestone 2:

- richer zone policy;
- private/public leakage hardening;
- peer reports;
- reachability graph;
- probe/sentinel agent.

Milestone 3:

- signed snapshots;
- dashboard;
- metrics;
- 200-agent load test.

Milestone 4:

- bootnode exports;
- DNS discovery material;
- optional devp2p integration.

Some placeholder packages and documents already exist for these later milestones, but the active supported MVP is Milestone 1.

## Troubleshooting

| Problem | Cause | Fix |
| --- | --- | --- |
| Agent cannot connect to IPC. | Wrong datadir or permissions. | Check `geth.ipc` path and user. |
| Heartbeat rejected wrong `network_id`. | Geth is on the wrong network. | Check `--networkid 702`. |
| Heartbeat rejected wrong `chain_id`. | Genesis config chain ID mismatch. | Check genesis `chainId` is `702` / `0x2be`. |
| Enode advertises `127.0.0.1` for a public node. | NAT advertisement is wrong. | Start Geth with `--nat extip:PUBLIC_IP`. |
| TCP unreachable. | Firewall/security group blocks P2P. | Open `30308/tcp`. |
| `admin_addPeer` returns true but peer count stays low. | Wrong chain, incompatible caps, firewall, or remote rejection. | Check Geth logs and `admin.peers`. |
