package gethclient

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/uzh/uzh-peer-oracle/internal/config"
)

func TestFakeIPCCollectHeartbeatAndAddPeer(t *testing.T) {
	nodeID := strings.Repeat("a", 128)
	addedPeer := ""
	client := NewWithDialer(func(ctx context.Context) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()
		go func() {
			defer serverConn.Close()
			var req rpcRequest
			if err := json.NewDecoder(serverConn).Decode(&req); err != nil {
				t.Errorf("decode request: %v", err)
				return
			}
			var result any
			switch req.Method {
			case "admin_nodeInfo":
				result = NodeInfo{
					ID:    nodeID,
					Name:  "Geth/v1.10.26-stable-e5eb32ac/linux-amd64/go1.18.5",
					Enode: "enode://" + nodeID + "@127.0.0.1:30308",
					ENR:   "enr:-fake",
				}
			case "admin_peers":
				result = []Peer{}
			case "net_version":
				result = "702"
			case "eth_chainId":
				result = "0x2be"
			case "eth_blockNumber":
				result = "0x2a"
			case "net_peerCount":
				result = "0x0"
			case "web3_clientVersion":
				result = "Geth/v1.10.26-stable-e5eb32ac/linux-amd64/go1.18.5"
			case "eth_getBlockByNumber":
				result = map[string]any{"hash": "0xgenesis"}
			case "admin_addPeer":
				var params []string
				raw, _ := json.Marshal(req.Params)
				_ = json.Unmarshal(raw, &params)
				if len(params) == 1 {
					addedPeer = params[0]
				}
				result = true
			default:
				t.Errorf("unexpected method %s", req.Method)
				result = nil
			}
			raw, _ := json.Marshal(result)
			_ = json.NewEncoder(serverConn).Encode(rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: raw})
		}()
		return clientConn, nil
	})

	cfg := config.Defaults()
	cfg.Agent.NodeName = "student-1"
	cfg.Geth.ExpectedNetworkID = "702"
	cfg.Geth.ExpectedChainID = "0x2be"
	hb, peers, err := CollectHeartbeat(context.Background(), client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if hb.NodeID != nodeID || hb.NetworkID != "702" || hb.ChainID != "0x2be" || hb.BlockNumber != 42 {
		t.Fatalf("unexpected heartbeat: %+v", hb)
	}
	if len(peers) != 0 {
		t.Fatalf("peers=%d want=0", len(peers))
	}

	enode := "enode://" + strings.Repeat("b", 128) + "@127.0.0.1:30308"
	ok, err := client.AddPeer(context.Background(), enode)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || addedPeer != enode {
		t.Fatalf("addPeer ok=%v addedPeer=%q", ok, addedPeer)
	}
}
