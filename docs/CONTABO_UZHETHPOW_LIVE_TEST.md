# Contabo UZHETHPOW Live Test

This runbook is for the real Contabo full node:

- public IP: `157.173.125.128`
- Geth IPC: `/home/ethereum/uzhethereum/blockchain/geth.ipc`
- Geth P2P: `30303`
- reth occupies: `30308`

## Build

```bash
go test ./...
make build
```

## Start oracle

```bash
export UZH_PEER_ORACLE_TOKEN="change-me-long-random"

./bin/uzh-peer-oracle server \
  --config configs/uzhethpow.contabo.oracle.yml \
  --seed configs/uzhethpow.contabo.peers.txt
```

## Health

```bash
curl http://127.0.0.1:8787/health
```

## Debug nodes

```bash
curl -H "Authorization: Bearer $UZH_PEER_ORACLE_TOKEN" \
  http://127.0.0.1:8787/v1/debug/nodes
```

## Contabo doctor

```bash
./bin/uzh-peer-oracle contabo-doctor \
  --config configs/uzhethpow.contabo.agent.yml
```

Copy the printed line:

```yaml
expected_genesis_hash: "<detected hash>"
```

Keep it for the later hardening pass. Do not force it yet in this live run.

## Measure before

```bash
./scripts/contabo_peer_perf.sh 60 5
```

## Agent once

```bash
./bin/uzh-peer-oracle agent \
  --config configs/uzhethpow.contabo.agent.yml \
  --once
```

## Measure after

```bash
./scripts/contabo_peer_perf.sh 120 5
```

## Continuous agent

```bash
./bin/uzh-peer-oracle agent \
  --config configs/uzhethpow.contabo.agent.yml
```

## Success criteria

- oracle starts
- seed file ingests
- Contabo node heartbeats
- `/v1/debug/nodes` shows the Contabo node
- `/v1/peers` returns public peers
- agent attempts `admin_addPeer`
- `/v1/peer-report` receives results
- reachability edge is updated
- `admin.peers.length` increases, or the failure reason is explicit
- no `static-nodes.json` is touched
- private `172.23.x` peers are not returned to a public requester
