package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uzh/uzh-peer-oracle/internal/gethclient"
	"github.com/uzh/uzh-peer-oracle/internal/log"
)

func TestContaboDoctorRendersGenesisHashWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "geth.ipc")
	if err := os.WriteFile(socketPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	oldFactory := newDoctorClient
	oldListenerCheck := detectPublicHTTPRPC
	newDoctorClient = func(ipcPath string) doctorClient {
		return fakeDoctorClient{}
	}
	detectPublicHTTPRPC = func(ctx context.Context, patterns []string) (bool, string) {
		return false, "127.0.0.1:8545"
	}
	defer func() {
		newDoctorClient = oldFactory
		detectPublicHTTPRPC = oldListenerCheck
	}()

	cfgText := `
agent:
  oracle_url: "http://127.0.0.1:8787"
geth:
  ipc_path: "` + socketPath + `"
  expected_network_id: "702"
  expected_chain_id: "0x2be"
  public_ip: "157.173.125.128"
  p2p_tcp_port: 30303
  p2p_udp_port: 30303
`
	cfgPath := filepath.Join(dir, "agent.yml")
	if err := os.WriteFile(cfgPath, []byte(cfgText), 0o600); err != nil {
		t.Fatal(err)
	}

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldStdout }()

	if err := runContaboDoctor(context.Background(), log.New(), []string{"--config", cfgPath}); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	if !strings.Contains(output, "detected genesis hash: 0xfeedbeef") {
		t.Fatalf("doctor output missing genesis hash:\n%s", output)
	}
	if !strings.Contains(output, "expected_genesis_hash: \"0xfeedbeef\"") {
		t.Fatalf("doctor output missing ready-to-paste config line:\n%s", output)
	}
}

type fakeDoctorClient struct{}

func (fakeDoctorClient) NodeInfo(ctx context.Context) (*gethclient.NodeInfo, error) {
	return &gethclient.NodeInfo{
		ID:    strings.Repeat("a", 128),
		Name:  "Geth/v1.10.26-stable-e5eb32ac/linux-amd64/go1.18.5",
		Enode: "enode://" + strings.Repeat("a", 128) + "@157.173.125.128:30303",
		ENR:   "enr:-contabo",
	}, nil
}

func (fakeDoctorClient) NetVersion(ctx context.Context) (string, error) {
	return "702", nil
}

func (fakeDoctorClient) ChainID(ctx context.Context) (string, error) {
	return "0x2be", nil
}

func (fakeDoctorClient) GenesisHash(ctx context.Context) (string, error) {
	return "0xfeedbeef", nil
}

func (fakeDoctorClient) BlockNumber(ctx context.Context) (uint64, error) {
	return 42, nil
}

func (fakeDoctorClient) Syncing(ctx context.Context) (*gethclient.SyncingStatus, error) {
	return &gethclient.SyncingStatus{Syncing: false}, nil
}

func (fakeDoctorClient) PeerCount(ctx context.Context) (int, error) {
	return 3, nil
}

func (fakeDoctorClient) AdminPeers(ctx context.Context) ([]gethclient.Peer, error) {
	return nil, nil
}
