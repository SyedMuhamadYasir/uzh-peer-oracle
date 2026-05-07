package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/agent"
	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/db"
	"github.com/uzh/uzh-peer-oracle/internal/gethclient"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
	"github.com/uzh/uzh-peer-oracle/internal/server"
	"github.com/uzh/uzh-peer-oracle/internal/wireprobe"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := log.New()
	var err error
	switch os.Args[1] {
	case "server":
		err = runServer(ctx, logger, os.Args[2:])
	case "agent":
		err = runAgent(ctx, logger, os.Args[2:])
	case "ingest":
		err = runIngest(ctx, logger, os.Args[2:])
	case "diagnose":
		err = runDiagnose(ctx, logger, os.Args[2:])
	case "wire-probe":
		err = runWireProbe(ctx, os.Args[2:])
	case "diagnose-peer":
		err = runDiagnosePeer(ctx, os.Args[2:])
	case "explain-peer-failure":
		err = runExplainPeerFailure(ctx, os.Args[2:])
	case "probe", "export-bootnodes", "export-dns", "snapshot", "load-test":
		err = fmt.Errorf("%s is reserved for Milestones 2-4; Milestone 1 implements server, ingest, diagnose, and agent", os.Args[1])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runServer(ctx context.Context, logger *log.Logger, args []string) error {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	seedPath := fs.String("seed", "", "seed peer file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	app, err := server.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()
	path := firstNonEmpty(*seedPath, cfg.Seed.Path)
	if path != "" {
		if _, err := app.IngestSeedFile(ctx, path); err != nil {
			return err
		}
	}
	return app.Run(ctx)
}

func runAgent(ctx context.Context, logger *log.Logger, args []string) error {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	r, err := agent.New(cfg, logger)
	if err != nil {
		return err
	}
	return r.Run(ctx)
}

func runIngest(ctx context.Context, logger *log.Logger, args []string) error {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	seedPath := fs.String("seed", "", "seed peer file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	app, err := server.New(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer app.Close()
	path := firstNonEmpty(*seedPath, cfg.Seed.Path)
	if path == "" {
		return fmt.Errorf("--seed or seed.path is required")
	}
	res, err := app.IngestSeedFile(ctx, path)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(b))
	return nil
}

func runDiagnose(ctx context.Context, logger *log.Logger, args []string) error {
	fs := flag.NewFlagSet("diagnose", flag.ExitOnError)
	configPath := fs.String("config", "", "config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	diagCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	hb, peers, err := gethclient.CollectHeartbeat(diagCtx, gethclient.New(cfg.Geth.IPCPath), cfg)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(map[string]any{
		"ipc_path":       cfg.Geth.IPCPath,
		"node_id":        hb.NodeID,
		"enode":          hb.Enode,
		"enr":            hb.ENR,
		"network_id":     hb.NetworkID,
		"chain_id":       hb.ChainID,
		"genesis_hash":   hb.GenesisHash,
		"block_number":   hb.BlockNumber,
		"peer_count":     hb.PeerCount,
		"admin_peers":    len(peers),
		"client_version": hb.ClientVersion,
	}, "", "  ")
	fmt.Println(string(b))
	if cfg.Geth.ExpectedNetworkID != "" && hb.NetworkID != cfg.Geth.ExpectedNetworkID {
		return fmt.Errorf("wrong network_id: got %s expected %s", hb.NetworkID, cfg.Geth.ExpectedNetworkID)
	}
	if cfg.Geth.ExpectedChainID != "" && !strings.EqualFold(hb.ChainID, cfg.Geth.ExpectedChainID) {
		return fmt.Errorf("wrong chain_id: got %s expected %s", hb.ChainID, cfg.Geth.ExpectedChainID)
	}
	return nil
}

func runWireProbe(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("wire-probe", flag.ExitOnError)
	enodeRaw := fs.String("enode", "", "enode URL")
	enrRaw := fs.String("enr", "", "ENR record")
	timeout := fs.Duration("timeout", 5*time.Second, "timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	prober := wireprobe.New(config.WireProbeConfig{
		Enabled:        true,
		TCPCheck:       true,
		DiscoveryCheck: true,
		RLPxCheck:      true,
		EthStatusCheck: false,
		TimeoutSeconds: int(timeout.Seconds()),
		MaxParallel:    1,
	})
	probeCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	var res oracle.WireProbeResult
	if strings.TrimSpace(*enrRaw) != "" {
		res = prober.ProbeENR(probeCtx, *enrRaw)
	} else {
		res = prober.ProbeEnode(probeCtx, *enodeRaw)
	}
	return json.NewEncoder(os.Stdout).Encode(res)
}

func runDiagnosePeer(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("diagnose-peer", flag.ExitOnError)
	configPath := fs.String("config", "", "agent config file")
	enodeRaw := fs.String("enode", "", "target enode")
	enrRaw := fs.String("enr", "", "target enr")
	timeout := fs.Duration("timeout", 5*time.Second, "timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	prober := wireprobe.New(cfg.WireProbe)
	probeCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	var res oracle.WireProbeResult
	if strings.TrimSpace(*enrRaw) != "" {
		res = prober.ProbeENR(probeCtx, *enrRaw)
	} else {
		res = prober.ProbeEnode(probeCtx, *enodeRaw)
	}
	hb, peers, err := gethclient.CollectHeartbeat(probeCtx, gethclient.New(cfg.Geth.IPCPath), cfg)
	if err != nil {
		return err
	}
	alreadyConnected := false
	if res.NodeID != "" {
		_, alreadyConnected = gethclient.FindPeer(peers, res.NodeID)
	}
	out := map[string]any{
		"local_node_id":     hb.NodeID,
		"local_peer_count":  hb.PeerCount,
		"target_probe":      res,
		"already_connected": alreadyConnected,
		"explanation":       wireprobe.Explain(res, hb.PeerCount, cfg.Agent.TargetPeers, alreadyConnected),
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	return nil
}

func runExplainPeerFailure(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("explain-peer-failure", flag.ExitOnError)
	configPath := fs.String("config", "configs/oracle.example.yml", "oracle config file")
	fromNodeID := fs.String("node-id", "", "source node ID")
	targetNodeID := fs.String("target-node-id", "", "target node ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *fromNodeID == "" || *targetNodeID == "" {
		return fmt.Errorf("--node-id and --target-node-id are required")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	store, err := db.Open(ctx, cfg.Storage)
	if err != nil {
		return err
	}
	defer store.Close()
	edge, edgeErr := store.GetReachabilityEdge(ctx, *fromNodeID, *targetNodeID)
	probe, probeErr := store.LatestWireProbeResult(ctx, *targetNodeID)
	targetNode, nodeErr := store.GetNode(ctx, *targetNodeID)
	parts := []string{}
	switch {
	case probeErr == nil && !probe.ParseOK:
		parts = append(parts, "invalid enode")
	case targetNode != nil && (strings.HasPrefix(targetNode.IP, "127.") || strings.HasPrefix(targetNode.IP, "10.") || strings.HasPrefix(targetNode.IP, "192.168.") || strings.HasPrefix(targetNode.IP, "172.16.")):
		parts = append(parts, "private/loopback advertised IP")
	}
	if probeErr == nil {
		switch {
		case !probe.TCPOK && probe.TCPStatus == "error":
			parts = append(parts, "TCP closed")
		case probe.TCPOK && probe.RLPxStatus == "error":
			parts = append(parts, "TCP reachable but RLPx failed")
		case probe.RLPxOK && !probe.EthStatusOK && probe.EthStatusStatus == "error":
			parts = append(parts, "RLPx works but eth protocol missing or wrong network/genesis")
		}
	}
	if edgeErr == nil && edge != nil && edge.FailureCount > 0 {
		parts = append(parts, edge.LastError)
	}
	if len(parts) == 0 {
		parts = append(parts, "no strong stored failure reason; check latest wire probe and peer reports")
	}
	out := map[string]any{
		"from_node_id":   *fromNodeID,
		"target_node_id": *targetNodeID,
		"explanation":    strings.Join(parts, "; "),
		"wire_probe":     nilIfProbeErr(probe, probeErr),
		"reachability":   nilIfEdgeErr(edge, edgeErr),
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
	if probeErr != nil && probeErr != sql.ErrNoRows && nodeErr != nil && edgeErr != nil {
		return fmt.Errorf("no peer evidence found in DB")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func usage() {
	fmt.Println(`uzh-peer-oracle commands:
  server --config configs/oracle.example.yml --seed configs/peers.example.txt
  ingest --config configs/oracle.example.yml --seed configs/peers.example.txt
  diagnose --config configs/agent.example.yml
  agent --config configs/agent.example.yml
  wire-probe --enode "enode://..." --timeout 5s
  wire-probe --enr "enr:..." --timeout 5s
  diagnose-peer --config configs/agent.example.yml --enode "enode://..."
  explain-peer-failure --config configs/oracle.example.yml --node-id A --target-node-id B

Future Milestones 2-4 reserve probe, export-bootnodes, export-dns, snapshot, and load-test.`)
}

func nilIfProbeErr(probe *oracle.WireProbeResult, err error) any {
	if err != nil {
		return nil
	}
	return probe
}

func nilIfEdgeErr(edge *oracle.ReachabilityEdge, err error) any {
	if err != nil {
		return nil
	}
	return edge
}
