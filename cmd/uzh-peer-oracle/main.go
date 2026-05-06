package main

import (
	"context"
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
	"github.com/uzh/uzh-peer-oracle/internal/gethclient"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/server"
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

Future Milestones 2-4 reserve probe, export-bootnodes, export-dns, snapshot, and load-test.`)
}
