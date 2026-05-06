# Professor Demo

Status: roadmap demo. For the current Milestone 1 demo, show the oracle server, seed ingestion, local IPC diagnose, two agents, `/v1/debug/nodes`, `/v1/peers`, and Geth `net.peerCount` increasing through `admin_addPeer`.

## Problem

Geth bootnodes do not guarantee useful private-testnet peering. In a teaching network, nodes may sit on the public internet, UZH campus, UZH VPN, or lab-only private networks. Native discovery may be filtered, polluted, stale, or unaware of the topology.

## Solution

UZH Peer Oracle is a topology-aware rendezvous service. Agents collect local Geth identity and chain metadata over IPC, heartbeat to the oracle, receive compatible reachable peers, and call local `admin_addPeer`.

No Geth patching. No Geth upgrade. No public admin RPC. No manually maintained `static-nodes.json`.

## Academic Angle

- Signed peer snapshots.
- Reachability graph.
- Anti-eclipse peer selection.
- Zone-aware privacy.
- Adaptive peer matchmaking.
- Observability dashboard and metrics.
- Compatibility with legacy Geth `1.10.26`.

## Demo Script

1. Start the oracle:

```bash
export UZH_PEER_ORACLE_TOKEN="long-random-token"
uzh-peer-oracle server --config configs/oracle.example.yml --seed configs/peers.example.txt
```

2. Show seed ingestion output:

```bash
uzh-peer-oracle ingest --config configs/oracle.example.yml --seed configs/peers.example.txt
```

3. Start two public Geth nodes with local agents.

4. Start one VPN/internal node and one VPN probe.

5. Show public recommendations:

```bash
curl -H "Authorization: Bearer $UZH_PEER_ORACLE_TOKEN" \
  "http://127.0.0.1:8787/v1/peers?node_id=<public-node-id>"
```

The public node receives public peers only.

6. Show VPN recommendations:

```bash
curl -H "Authorization: Bearer $UZH_PEER_ORACLE_TOKEN" \
  "http://127.0.0.1:8787/v1/peers?node_id=<vpn-node-id>"
```

The VPN node receives public plus VPN peers when authorized/classified.

7. Show the Milestone 1 registry and recommendations:

```bash
curl -H "Authorization: Bearer $UZH_PEER_ORACLE_TOKEN" \
  http://127.0.0.1:8787/v1/debug/nodes
```

8. Future Milestone 3 demo: show dashboard metrics and signed snapshot verification.

```bash
curl http://127.0.0.1:8787/v1/snapshot/public > snapshot.json
uzh-peer-oracle snapshot verify --config configs/agent.example.yml --file snapshot.json
```

9. Show Geth `net.peerCount` increasing without creating or modifying `static-nodes.json`.

## Closing Line

The project keeps the operational simplicity of a summer-school network while adding systems-grade verification, topology awareness, auditability, and adaptive peer selection.
