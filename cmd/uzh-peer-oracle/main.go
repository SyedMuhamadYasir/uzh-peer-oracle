package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/agent"
	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/db"
	"github.com/uzh/uzh-peer-oracle/internal/enodeutil"
	"github.com/uzh/uzh-peer-oracle/internal/gethclient"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
	"github.com/uzh/uzh-peer-oracle/internal/server"
	"github.com/uzh/uzh-peer-oracle/internal/wireprobe"
)

type doctorClient interface {
	NodeInfo(ctx context.Context) (*gethclient.NodeInfo, error)
	NetVersion(ctx context.Context) (string, error)
	ChainID(ctx context.Context) (string, error)
	GenesisHash(ctx context.Context) (string, error)
	BlockNumber(ctx context.Context) (uint64, error)
	Syncing(ctx context.Context) (*gethclient.SyncingStatus, error)
	PeerCount(ctx context.Context) (int, error)
	AdminPeers(ctx context.Context) ([]gethclient.Peer, error)
}

var newDoctorClient = func(ipcPath string) doctorClient {
	return gethclient.New(ipcPath)
}

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
	case "contabo-doctor":
		err = runContaboDoctor(ctx, logger, os.Args[2:])
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
	once := fs.Bool("once", false, "run one heartbeat/peering cycle and exit")
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
	if *once {
		return r.RunOnce(ctx)
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

func runContaboDoctor(ctx context.Context, logger *log.Logger, args []string) error {
	fs := flag.NewFlagSet("contabo-doctor", flag.ExitOnError)
	configPath := fs.String("config", "", "agent config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	_ = logger
	diagCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	client := newDoctorClient(cfg.Geth.IPCPath)
	if _, err := os.Stat(cfg.Geth.IPCPath); err != nil {
		fmt.Printf("IPC exists: no (%v)\n", err)
	} else {
		fmt.Println("IPC exists: yes")
	}
	info, infoErr := client.NodeInfo(diagCtx)
	if infoErr != nil {
		fmt.Printf("admin_nodeInfo: error (%v)\n", infoErr)
		return fmt.Errorf("admin_nodeInfo failed: %w", infoErr)
	}
	fmt.Println("admin_nodeInfo: ok")
	fmt.Printf("own enode: %s\n", info.Enode)
	if strings.TrimSpace(info.ENR) != "" {
		fmt.Printf("own ENR: %s\n", info.ENR)
	} else {
		fmt.Println("own ENR: <not reported>")
	}
	networkID, _ := client.NetVersion(diagCtx)
	chainID, _ := client.ChainID(diagCtx)
	genesisHash, _ := client.GenesisHash(diagCtx)
	blockNumber, _ := client.BlockNumber(diagCtx)
	syncing, syncingErr := client.Syncing(diagCtx)
	peerCount, _ := client.PeerCount(diagCtx)
	adminPeers, _ := client.AdminPeers(diagCtx)
	fmt.Printf("net_version: %s\n", networkID)
	fmt.Printf("eth_chainId: %s\n", chainID)
	fmt.Printf("detected genesis hash: %s\n", firstNonEmpty(genesisHash, "<unavailable>"))
	fmt.Printf("expected_genesis_hash: %q\n", genesisHash)
	fmt.Printf("eth_blockNumber: %d\n", blockNumber)
	if syncingErr != nil {
		fmt.Printf("eth_syncing: unavailable (%v)\n", syncingErr)
	} else if syncing == nil || !syncing.Syncing {
		fmt.Println("eth_syncing: false")
	} else {
		b, _ := json.Marshal(syncing.State)
		fmt.Printf("eth_syncing: %s\n", string(b))
	}
	fmt.Printf("net.peerCount: %d\n", peerCount)
	fmt.Printf("admin.peers.length: %d\n", len(adminPeers))
	fmt.Printf("configured public IP: %s\n", firstNonEmpty(cfg.Geth.PublicIP, "<empty>"))
	fmt.Printf("configured P2P TCP/UDP: %d / %d\n", cfg.Geth.P2PTCPPort, cfg.Geth.P2PUDPPort)
	if parsed, err := gethclientParseEnode(info.Enode); err != nil {
		fmt.Printf("advertised enode parse: error (%v)\n", err)
	} else {
		fmt.Printf("advertised enode IP: %s\n", firstNonEmpty(parsed.IP, parsed.Host))
		fmt.Printf("advertised TCP/UDP ports: %d / %d\n", parsed.TCPPort, parsed.UDPPort)
		if isPrivateOrLoopback(parsed.IP, parsed.Host) {
			fmt.Println("warning: advertised enode IP is private or loopback; public peers will not be able to use it reliably")
			fmt.Println("suggested fix: start geth with --nat extip:157.173.125.128")
		}
		if cfg.Geth.PublicIP != "" && parsed.IP != "" && !strings.EqualFold(parsed.IP, cfg.Geth.PublicIP) {
			fmt.Printf("warning: configured public IP %s differs from advertised enode IP %s\n", cfg.Geth.PublicIP, parsed.IP)
			fmt.Println("suggested fix: align geth --nat extip and agent geth.public_ip")
		}
		if cfg.Geth.P2PTCPPort != 0 && parsed.TCPPort != 0 && cfg.Geth.P2PTCPPort != parsed.TCPPort {
			fmt.Printf("warning: configured P2P TCP port %d differs from advertised enode TCP port %d\n", cfg.Geth.P2PTCPPort, parsed.TCPPort)
			fmt.Println("suggested fix: set geth.p2p_tcp_port to the actual advertised Geth port")
		}
		if cfg.Geth.P2PUDPPort != 0 && parsed.UDPPort != 0 && cfg.Geth.P2PUDPPort != parsed.UDPPort {
			fmt.Printf("warning: configured P2P UDP port %d differs from advertised enode UDP port %d\n", cfg.Geth.P2PUDPPort, parsed.UDPPort)
			fmt.Println("suggested fix: set geth.p2p_udp_port to the actual advertised Geth port")
		}
	}
	fmt.Println("warning: this Contabo Geth uses 30303 while reth occupies 30308; keep the agent on 30303")
	fmt.Println("note: uzh-peer-oracle uses local IPC only and does not need public HTTP RPC")
	if warn, detail := detectPublicHTTPRPC(ctx, []string{"*:8545", "0.0.0.0:8545", "[::]:8545"}); warn {
		fmt.Printf("warning: geth HTTP RPC appears to be listening publicly (%s)\n", detail)
		fmt.Println("suggested fix: bind geth HTTP RPC to 127.0.0.1 with --http.addr 127.0.0.1")
	} else if detail != "" {
		fmt.Printf("HTTP RPC listener check: %s\n", detail)
	}
	if networkID != "" && cfg.Geth.ExpectedNetworkID != "" && networkID != cfg.Geth.ExpectedNetworkID {
		fmt.Printf("warning: runtime network_id %s differs from config %s\n", networkID, cfg.Geth.ExpectedNetworkID)
	}
	if chainID != "" && cfg.Geth.ExpectedChainID != "" && !strings.EqualFold(chainID, cfg.Geth.ExpectedChainID) {
		fmt.Printf("warning: runtime chain_id %s differs from config %s\n", chainID, cfg.Geth.ExpectedChainID)
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
  contabo-doctor --config configs/uzhethpow.contabo.agent.yml
  agent --config configs/agent.example.yml [--once]
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

type parsedEnode struct {
	Host    string
	IP      string
	TCPPort int
	UDPPort int
}

func gethclientParseEnode(raw string) (*parsedEnode, error) {
	parsed, err := enodeutil.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &parsedEnode{Host: parsed.Host, IP: parsed.IP, TCPPort: parsed.TCPPort, UDPPort: parsed.UDPPort}, nil
}

func isPrivateOrLoopback(ip, host string) bool {
	target := strings.TrimSpace(ip)
	if target == "" {
		target = strings.TrimSpace(host)
	}
	parsed := net.ParseIP(target)
	if parsed == nil {
		return false
	}
	return parsed.IsPrivate() || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified()
}

var detectPublicHTTPRPC = func(ctx context.Context, patterns []string) (bool, string) {
	ssPath, err := exec.LookPath("ss")
	if err != nil {
		return false, "ss not found; unable to inspect listeners"
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(checkCtx, ssPath, "-lnt").CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("ss failed: %v", err)
	}
	text := string(out)
	for _, p := range patterns {
		if strings.Contains(text, p) {
			return true, p
		}
	}
	if strings.Contains(text, "127.0.0.1:8545") {
		return false, "127.0.0.1:8545"
	}
	return false, "no public 8545 listener detected"
}
