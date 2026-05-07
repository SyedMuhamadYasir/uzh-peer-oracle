package wireprobe

import (
	"context"
	"crypto/ecdsa"
	"net"
	"strconv"
	"testing"

	gethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/uzh/uzh-peer-oracle/internal/config"
)

func TestProbeEnodeParseAndTCP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	prober := New(config.WireProbeConfig{Enabled: true, TCPCheck: true, TimeoutSeconds: 2})
	port := ln.Addr().(*net.TCPAddr).Port
	res := prober.ProbeEnode(context.Background(), mustEnode(t, "127.0.0.1", port, port))
	if !res.ParseOK {
		t.Fatalf("expected parse OK: %+v", res)
	}
	if !res.TCPOK {
		t.Fatalf("expected TCP OK: %+v", res)
	}
	if res.IPClass != "loopback" {
		t.Fatalf("expected loopback classification: %+v", res)
	}
}

func TestProbeENRParse(t *testing.T) {
	key, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	record := mustENR(t, key)
	prober := New(config.WireProbeConfig{Enabled: true, TCPCheck: false, TimeoutSeconds: 2})
	res := prober.ProbeENR(context.Background(), record)
	if !res.ParseOK || res.NodeID == "" {
		t.Fatalf("expected ENR parse OK: %+v", res)
	}
	if res.TCPPort != 30308 || res.UDPPort != 30308 {
		t.Fatalf("expected ports from ENR: %+v", res)
	}
}

func TestProbeTCPFailure(t *testing.T) {
	prober := New(config.WireProbeConfig{Enabled: true, TCPCheck: true, TimeoutSeconds: 1})
	res := prober.ProbeEnode(context.Background(), mustEnode(t, "127.0.0.1", 9, 9))
	if !res.ParseOK {
		t.Fatalf("expected parse OK: %+v", res)
	}
	if res.TCPOK {
		t.Fatalf("expected TCP failure: %+v", res)
	}
	if res.TCPStatus != "error" {
		t.Fatalf("expected tcp error status: %+v", res)
	}
}

func mustENR(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	db, err := enode.OpenDB("")
	if err != nil {
		t.Fatal(err)
	}
	node := enode.NewLocalNode(db, key)
	node.SetStaticIP(net.ParseIP("127.0.0.1"))
	node.Set(enr.TCP(30308))
	node.SetFallbackUDP(30308)
	return node.Node().String()
}

func mustEnode(t *testing.T, ip string, tcpPort, udpPort int) string {
	t.Helper()
	key, err := gethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return enode.NewV4(&key.PublicKey, net.ParseIP(ip), tcpPort, udpPort).URLv4()
}

func itoa(v int) string {
	return strconv.Itoa(v)
}
