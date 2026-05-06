# Threat Model

Status: roadmap threat model. Milestone 1 implements the bearer-token, local IPC, SQLite, seed validation, and basic wrong-network/wrong-chain controls. Probe-based reachability, signed snapshots, and richer zone policy are future milestones.

## Goals

- Do not expose Geth admin RPC publicly.
- Do not leak private/VPN peers to public requesters.
- Reject wrong network and wrong chain peers.
- Reduce peer poisoning.
- Make registry state auditable with signed snapshots.
- Keep operation simple enough for a teaching network.

## Non-Goals

- This is not a consensus layer.
- This is not a replacement for Ethereum node security.
- This does not make a malicious peer safe to connect to.
- This does not hide peer IPs from authorized requesters.

## Controls

- Bearer token authentication for write and recommendation APIs.
- Optional per-node/per-zone tokens.
- Source CIDR and token constrained zone classification.
- Private peers excluded from public bootnode and DNS exports.
- Network ID, chain ID, and optional genesis hash verification.
- Agent uses local IPC only.
- Agent never removes unmanaged peers.
- Ed25519 signed snapshots.
- Audit events for seed ingestion and rejects.
- Request body limits, timeouts, and rate limiting.

## Residual Risks

- A shared class token can be leaked. For production, use per-node tokens.
- A malicious authorized node can submit false hints. The oracle constrains hints and improves confidence through probes and peer reports.
- TCP reachability does not prove useful Ethereum protocol compatibility. Agents therefore check `admin_peers` and `eth/*` caps after `admin_addPeer`.
- DNS discovery publication must be carefully scoped to public peers.
