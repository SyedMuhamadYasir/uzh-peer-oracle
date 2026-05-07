package unit

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/matcher"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
	"github.com/uzh/uzh-peer-oracle/internal/seed"
)

func TestContaboOracleConfigUses30303(t *testing.T) {
	cfg, err := config.Load("/mnt/d/Download/Academic Stuff/PhD/Codex Projects/uzh-peer-oracle/configs/uzhethpow.contabo.oracle.yml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Network.DefaultTCPPort != 30303 || cfg.Network.DefaultUDPPort != 30303 {
		t.Fatalf("unexpected ports: tcp=%d udp=%d", cfg.Network.DefaultTCPPort, cfg.Network.DefaultUDPPort)
	}
}

func TestContaboSeedParsesAllSixPeers(t *testing.T) {
	cfg := config.Defaults()
	f, err := os.Open("/mnt/d/Download/Academic Stuff/PhD/Codex Projects/uzh-peer-oracle/configs/uzhethpow.contabo.peers.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	res := seed.Parse(f, cfg)
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected parse errors: %+v", res.Errors)
	}
	if len(res.Peers) != 6 {
		t.Fatalf("expected 6 peers, got %d", len(res.Peers))
	}
}

func TestPublicRequesterDoesNotReceiveContaboVPNPeers(t *testing.T) {
	cfg := config.Defaults()
	now := time.Now().UTC()
	nodes := []*oracle.Node{
		{NodeID: "self", Enode: "enode://aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa@157.173.125.128:30303", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "pub", Enode: "enode://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb@130.60.144.77:30308", Zones: []string{"public"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "vpn-a", Enode: "enode://cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc@172.23.72.147:30303", Zones: []string{"uzh-vpn"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
		{NodeID: "vpn-b", Enode: "enode://dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd@172.23.210.21:30303", Zones: []string{"uzh-vpn"}, NetworkID: "702", ChainID: "0x2be", LastSeen: now},
	}
	recs := matcher.Select(matcher.Request{
		Requester:      nodes[0],
		EffectiveZones: []string{"public"},
		Limit:          10,
		Now:            now,
	}, nodes, nil, cfg)
	for _, rec := range recs {
		if strings.HasPrefix(rec.IP, "172.23.") || rec.NodeID == "vpn-a" || rec.NodeID == "vpn-b" {
			t.Fatalf("private vpn peer leaked to public requester: %+v", rec)
		}
	}
}

func TestContaboPerfScriptUsesGethAttachExecSyntax(t *testing.T) {
	b, err := os.ReadFile("/mnt/d/Download/Academic Stuff/PhD/Codex Projects/uzh-peer-oracle/scripts/contabo_peer_perf.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(b)
	if !strings.Contains(script, "attach --exec") {
		t.Fatalf("expected geth attach --exec syntax in script, got:\n%s", script)
	}
	if !strings.Contains(script, "/home/ethereum/uzhethereum/geth") || !strings.Contains(script, "/home/ethereum/uzhethereum/blockchain/geth.ipc") {
		t.Fatalf("script missing contabo geth paths:\n%s", script)
	}
}
