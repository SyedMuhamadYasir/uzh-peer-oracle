# Student Mode

Current Contabo validation is **operator / hub-node mode**.

The local helper exists because the oracle cannot directly access another machine's local `geth.ipc`. For Contabo testing, the helper is useful because it can:

- read local Geth state over IPC
- call local `admin_addPeer`
- report back what actually happened

Students should **not** be expected to manually run a separate helper command in the final classroom UX.

Near-term student-facing directions are:

1. curated bootnodes / DNS export from the oracle
2. a future hidden wrapper that launches the helper automatically

For now, the helper is for operator-side validation and hub-node peering, not the final student classroom workflow.
