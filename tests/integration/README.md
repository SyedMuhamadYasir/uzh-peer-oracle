# Integration Tests

The integration acceptance tests require running Geth `1.10.26`, multiple datadirs, and reachable P2P ports. They are documented in the smoke scripts:

- `scripts/smoke_two_nodes.sh`
- `scripts/smoke_three_nodes.sh`
- `scripts/load_test_200_agents.sh`

The MVP acceptance path is:

1. Start the oracle.
2. Ingest a seed file.
3. Start two agents beside two Geth nodes.
4. Confirm `net.peerCount` rises without creating or modifying `static-nodes.json`.
5. Start a VPN probe and confirm public requesters do not receive VPN peers.
