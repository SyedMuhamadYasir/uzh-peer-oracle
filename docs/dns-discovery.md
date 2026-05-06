# DNS Discovery

Status: future Milestone 4 design note. DNS discovery is not part of the Milestone 1 runnable MVP.

DNS discovery is a secondary path. First make peering work with oracle recommendations and local `admin_addPeer`.

Future Milestone 4 will expose curated public material:

```bash
curl https://peer-oracle.example.uzh.ch/v1/bootnodes/enodes
curl https://peer-oracle.example.uzh.ch/v1/bootnodes/enrs
curl https://peer-oracle.example.uzh.ch/v1/dns/nodes.json
```

Curated means:

- correct `network_id`;
- correct `chain_id`;
- correct configured genesis hash;
- recent peer;
- verified public reachability;
- public IP only;
- no private/VPN peers leaked.

Planned workflow with the `devp2p` tool:

```bash
uzh-peer-oracle export-dns --config /etc/uzh-peer-oracle.yml > nodes.json
devp2p nodeset filter nodes.json > public.nodes
devp2p dns sign public.nodes --domain nodes.example.uzh.ch --key dns.key > enrtree.txt
devp2p dns to-cloudflare enrtree.txt
```

Then configure Geth:

```bash
--discovery.dns "enrtree://..."
```

Keep private/VPN trees separate and never publish them to public DNS unless the network owner explicitly wants that.
