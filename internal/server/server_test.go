package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

func TestMilestone1ServerAPI(t *testing.T) {
	ctx := context.Background()
	token := "test-token"
	t.Setenv("UZH_PEER_ORACLE_TOKEN", token)

	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Storage.SQLitePath = filepath.Join(dir, "oracle.db")
	cfg.Snapshot.Enabled = false

	app, err := New(ctx, cfg, log.New())
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	seedPath := filepath.Join(dir, "peers.txt")
	seedEnode := "enode://" + strings.Repeat("a", 128) + "@130.60.24.247:30308 # name=hub-public-1 zone=public role=hub"
	if err := os.WriteFile(seedPath, []byte(seedEnode+"\nnot-a-peer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := app.IngestSeedFile(ctx, seedPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Peers) != 1 || len(res.Errors) != 1 {
		t.Fatalf("unexpected seed result: peers=%d errors=%d", len(res.Peers), len(res.Errors))
	}

	ts := httptest.NewServer(app.Handler())
	defer ts.Close()

	health, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if health.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", health.StatusCode)
	}
	_ = health.Body.Close()

	noAuth, err := http.Get(ts.URL + "/v1/debug/nodes")
	if err != nil {
		t.Fatal(err)
	}
	if noAuth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("debug nodes without auth status=%d", noAuth.StatusCode)
	}
	_ = noAuth.Body.Close()

	nodeID := strings.Repeat("b", 128)
	hb := oracle.HeartbeatRequest{
		NodeName:      "student-1",
		Role:          "normal",
		NodeID:        nodeID,
		Enode:         "enode://" + nodeID + "@198.51.100.10:30308",
		IP:            "198.51.100.10",
		TCPPort:       30308,
		UDPPort:       30308,
		Zones:         []string{"public"},
		NetworkID:     "702",
		ChainID:       "0x2be",
		ClientVersion: "Geth/v1.10.26-stable-e5eb32ac/linux-amd64/go1.18.5",
		BlockNumber:   10,
	}
	var hbResp oracle.HeartbeatResponse
	doJSON(t, ts.URL+"/v1/heartbeat", token, hb, &hbResp)
	if !hbResp.Accepted || hbResp.Status != "verified" {
		t.Fatalf("heartbeat response: %+v", hbResp)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/v1/debug/nodes", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	debugResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer debugResp.Body.Close()
	if debugResp.StatusCode != http.StatusOK {
		t.Fatalf("debug nodes status=%d", debugResp.StatusCode)
	}
	var nodes []oracle.Node
	if err := json.NewDecoder(debugResp.Body).Decode(&nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Fatalf("nodes=%d want=2", len(nodes))
	}

	req, err = http.NewRequest(http.MethodGet, ts.URL+"/v1/peers?node_id="+nodeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	peersHTTP, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer peersHTTP.Body.Close()
	if peersHTTP.StatusCode != http.StatusOK {
		t.Fatalf("peers status=%d", peersHTTP.StatusCode)
	}
	var peers oracle.PeersResponse
	if err := json.NewDecoder(peersHTTP.Body).Decode(&peers); err != nil {
		t.Fatal(err)
	}
	if len(peers.Peers) != 1 || peers.Peers[0].NodeName != "hub-public-1" {
		t.Fatalf("unexpected peers: %+v", peers.Peers)
	}
}

func doJSON(t *testing.T, url, token string, in any, out any) {
	t.Helper()
	b, _ := json.Marshal(in)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s status=%d", url, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
}
