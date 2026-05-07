package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/gethclient"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type mockGeth struct {
	peers       []gethclient.Peer
	addedEnodes []string
	removed     []string
}

func (m *mockGeth) AddPeer(ctx context.Context, enode string) (bool, error) {
	m.addedEnodes = append(m.addedEnodes, enode)
	m.peers = []gethclient.Peer{{
		ID:    strings.Repeat("b", 128),
		Enode: enode,
		Caps:  []string{"eth/66"},
		Network: gethclient.PeerNetwork{
			RemoteAddress: "203.0.113.2:30308",
		},
		Protocols: map[string]any{"eth": map[string]any{}},
	}}
	return true, nil
}

func (m *mockGeth) AdminPeers(ctx context.Context) ([]gethclient.Peer, error) {
	return m.peers, nil
}

func (m *mockGeth) RemovePeer(ctx context.Context, enode string) (bool, error) {
	m.removed = append(m.removed, enode)
	return true, nil
}

func TestRunnerSendsPeerReportAfterAttempt(t *testing.T) {
	var reports []oracle.PeerReportRequest
	token := "test-token"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/v1/heartbeat":
			_ = json.NewEncoder(w).Encode(oracle.HeartbeatResponse{Accepted: true, Status: "verified", EffectiveZones: []string{"public"}, Score: 10})
		case r.URL.Path == "/v1/peers":
			_ = json.NewEncoder(w).Encode(oracle.PeersResponse{Peers: []oracle.PeerRecommendation{{
				NodeID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				Enode:  "enode://bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb@203.0.113.2:30308",
				Score:  18,
			}}})
		case r.URL.Path == "/v1/peer-report":
			var report oracle.PeerReportRequest
			_ = json.NewDecoder(r.Body).Decode(&report)
			reports = append(reports, report)
			_ = json.NewEncoder(w).Encode(map[string]any{"accepted": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := config.Defaults()
	cfg.Agent.OracleURL = ts.URL
	cfg.Agent.TargetPeers = 1
	cfg.LocalState.Path = t.TempDir() + "/agent-state.json"
	mock := &mockGeth{}
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	runner := &Runner{
		cfg:    cfg,
		geth:   mock,
		http:   ts.Client(),
		state:  &State{ManagedPeers: map[string]*ManagedPeer{}},
		logger: log.New(),
		token:  token,
		collectHeartbeat: func(ctx context.Context) (*oracle.HeartbeatRequest, []gethclient.Peer, error) {
			return &oracle.HeartbeatRequest{
				NodeName:    "node-a",
				NodeID:      strings.Repeat("a", 128),
				Enode:       "enode://" + strings.Repeat("a", 128) + "@203.0.113.1:30308",
				IP:          "203.0.113.1",
				TCPPort:     30308,
				UDPPort:     30308,
				Zones:       []string{"public"},
				NetworkID:   "702",
				ChainID:     "0x2be",
				PeerCount:   0,
				BlockNumber: 1,
			}, nil, nil
		},
		sleep: func(time.Duration) {},
		now:   func() time.Time { return now },
	}
	if err := runner.Cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected one peer report, got %d", len(reports))
	}
	if !reports[0].AdminAddPeerResult || !reports[0].ObservedInAdminPeers || !reports[0].EthProtocolPresent {
		t.Fatalf("unexpected peer report: %+v", reports[0])
	}
}

func TestManagedPeerReplacementOnlyRemovesManagedPeers(t *testing.T) {
	cfg := config.Defaults()
	mock := &mockGeth{}
	runner := &Runner{
		cfg:  cfg,
		geth: mock,
		http: &http.Client{Timeout: time.Second},
		state: &State{ManagedPeers: map[string]*ManagedPeer{
			"managed": {NodeID: "managed", Enode: "enode://managed@203.0.113.3:30308", LastRecommendationScore: 1},
		}},
		logger: log.New(),
		now:    time.Now,
		sleep:  func(time.Duration) {},
	}
	peers := []gethclient.Peer{
		{ID: "managed", Enode: "enode://managed@203.0.113.3:30308"},
		{ID: "manual", Enode: "enode://manual@203.0.113.4:30308"},
	}
	recs := []oracle.PeerRecommendation{{NodeID: "better", Enode: "enode://better@203.0.113.5:30308", Score: 20}}
	replaced, err := runner.maybeReplaceManagedPeer(context.Background(), peers, recs)
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Fatal("expected replacement")
	}
	if len(mock.removed) != 1 || mock.removed[0] != "enode://managed@203.0.113.3:30308" {
		t.Fatalf("removed wrong peer: %+v", mock.removed)
	}
	if _, exists := runner.state.ManagedPeers["managed"]; exists {
		t.Fatal("managed peer should have been removed from state")
	}
}

func TestRunnerRunOnceExits(t *testing.T) {
	token := "test-token"
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		callCount++
		switch r.URL.Path {
		case "/v1/heartbeat":
			_ = json.NewEncoder(w).Encode(oracle.HeartbeatResponse{Accepted: true, Status: "verified", EffectiveZones: []string{"public"}, Score: 9})
		case "/v1/peers":
			_ = json.NewEncoder(w).Encode(oracle.PeersResponse{Peers: nil})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	cfg := config.Defaults()
	cfg.Agent.OracleURL = ts.URL
	cfg.LocalState.Path = t.TempDir() + "/agent-state.json"
	runner := &Runner{
		cfg:    cfg,
		geth:   &mockGeth{},
		http:   ts.Client(),
		state:  &State{ManagedPeers: map[string]*ManagedPeer{}},
		logger: log.New(),
		token:  token,
		collectHeartbeat: func(ctx context.Context) (*oracle.HeartbeatRequest, []gethclient.Peer, error) {
			return &oracle.HeartbeatRequest{
				NodeName:  "node-a",
				NodeID:    strings.Repeat("a", 128),
				Enode:     "enode://" + strings.Repeat("a", 128) + "@157.173.125.128:30303",
				IP:        "157.173.125.128",
				TCPPort:   30303,
				UDPPort:   30303,
				Zones:     []string{"public"},
				NetworkID: "702",
				ChainID:   "0x2be",
			}, nil, nil
		},
		sleep: func(time.Duration) {},
		now:   time.Now,
	}
	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if callCount != 2 {
		t.Fatalf("expected exactly heartbeat + peers calls, got %d", callCount)
	}
}
