package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/gethclient"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type Runner struct {
	cfg    *config.Config
	geth   *gethclient.Client
	http   *http.Client
	state  *State
	logger *log.Logger
	token  string
}

func New(cfg *config.Config, logger *log.Logger) (*Runner, error) {
	token := cfg.AgentToken()
	if err := config.RequireToken(token); err != nil {
		return nil, err
	}
	st, err := LoadState(cfg.LocalState.Path)
	if err != nil {
		return nil, err
	}
	return &Runner{
		cfg:    cfg,
		geth:   gethclient.New(cfg.Geth.IPCPath),
		http:   &http.Client{Timeout: 12 * time.Second},
		state:  st,
		logger: logger,
		token:  token,
	}, nil
}

func (r *Runner) Diagnose(ctx context.Context) error {
	hb, peers, err := gethclient.CollectHeartbeat(ctx, r.geth, r.cfg)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(map[string]any{
		"ipc_path":       r.cfg.Geth.IPCPath,
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
	if r.cfg.Geth.ExpectedNetworkID != "" && hb.NetworkID != r.cfg.Geth.ExpectedNetworkID {
		return fmt.Errorf("wrong network_id: got %s expected %s", hb.NetworkID, r.cfg.Geth.ExpectedNetworkID)
	}
	if r.cfg.Geth.ExpectedChainID != "" && !strings.EqualFold(hb.ChainID, r.cfg.Geth.ExpectedChainID) {
		return fmt.Errorf("wrong chain_id: got %s expected %s", hb.ChainID, r.cfg.Geth.ExpectedChainID)
	}
	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	r.logger.Info("agent started", map[string]any{"oracle": r.cfg.Agent.OracleURL, "ipc": r.cfg.Geth.IPCPath})
	for {
		if err := r.Cycle(ctx); err != nil {
			r.logger.Warn("agent cycle failed", map[string]any{"err": err.Error()})
		}
		select {
		case <-ctx.Done():
			return r.state.Save(r.cfg.LocalState.Path)
		case <-time.After(r.jittered(config.DurationSeconds(r.cfg.Agent.PeerRefreshIntervalSeconds, 20*time.Second))):
		}
	}
}

func (r *Runner) Cycle(ctx context.Context) error {
	cycleCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	hb, peers, err := gethclient.CollectHeartbeat(cycleCtx, r.geth, r.cfg)
	if err != nil {
		return fmt.Errorf("collect geth heartbeat: %w", err)
	}
	hbResp, err := r.heartbeat(cycleCtx, hb)
	if err != nil {
		return err
	}
	if !hbResp.Accepted {
		return fmt.Errorf("heartbeat rejected: %v", hbResp.RejectReason)
	}
	if hb.PeerCount >= r.cfg.Agent.TargetPeers {
		return r.state.Save(r.cfg.LocalState.Path)
	}
	resp, err := r.fetchPeers(cycleCtx, hb.NodeID)
	if err != nil {
		return err
	}
	current := map[string]bool{}
	for _, p := range peers {
		current[strings.ToLower(p.ID)] = true
	}
	added := 0
	for _, p := range resp.Peers {
		if added >= r.cfg.Agent.MaxManagedPeers {
			break
		}
		if p.NodeID == "" || p.Enode == "" || strings.EqualFold(p.NodeID, hb.NodeID) || current[strings.ToLower(p.NodeID)] {
			continue
		}
		if managed := r.state.ManagedPeers[p.NodeID]; managed != nil && time.Now().Before(managed.CooldownUntil) {
			continue
		}
		report := r.tryPeer(cycleCtx, hb.NodeID, p)
		r.logger.Info("peer attempt completed", map[string]any{
			"to_node_id":     report.ToNodeID,
			"admin_add_peer": report.AdminAddPeerResult,
			"observed":       report.ObservedInAdminPeers,
			"eth_protocol":   report.EthProtocolPresent,
			"error":          report.Error,
		})
		added++
	}
	return r.state.Save(r.cfg.LocalState.Path)
}

func (r *Runner) heartbeat(ctx context.Context, hb *oracle.HeartbeatRequest) (*oracle.HeartbeatResponse, error) {
	var resp oracle.HeartbeatResponse
	if err := r.postJSON(ctx, "/v1/heartbeat", hb, &resp); err != nil {
		return nil, err
	}
	r.logger.Info("heartbeat accepted", map[string]any{"status": resp.Status, "zones": resp.EffectiveZones, "score": resp.Score})
	return &resp, nil
}

func (r *Runner) fetchPeers(ctx context.Context, nodeID string) (*oracle.PeersResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(r.cfg.Agent.OracleURL, "/")+"/v1/peers?node_id="+nodeID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	res, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("peers request failed: %s %s", res.Status, string(b))
	}
	var out oracle.PeersResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *Runner) tryPeer(ctx context.Context, fromNodeID string, p oracle.PeerRecommendation) oracle.PeerReportRequest {
	now := time.Now().UTC()
	report := oracle.PeerReportRequest{FromNodeID: fromNodeID, ToNodeID: p.NodeID, ToEnode: p.Enode, AttemptedAt: now}
	managed := r.state.ManagedPeers[p.NodeID]
	if managed == nil {
		managed = &ManagedPeer{NodeID: p.NodeID, Enode: p.Enode}
		r.state.ManagedPeers[p.NodeID] = managed
	}
	managed.LastAttempt = now
	if r.cfg.Agent.DryRun {
		report.Error = "dry-run: admin_addPeer skipped"
		return report
	}
	ok, err := r.geth.AddPeer(ctx, p.Enode)
	report.AdminAddPeerResult = ok
	if err != nil {
		report.Error = err.Error()
		managed.FailureCount++
		managed.CooldownUntil = backoff(managed.FailureCount)
		return report
	}
	time.Sleep(5 * time.Second)
	peers, err := r.geth.AdminPeers(ctx)
	if err != nil {
		report.Error = err.Error()
		managed.FailureCount++
		managed.CooldownUntil = backoff(managed.FailureCount)
		return report
	}
	if peer, found := gethclient.FindPeer(peers, p.NodeID); found {
		report.ObservedInAdminPeers = true
		report.ConnectedAfterSeconds = true
		report.RemoteAddress = peer.Network.RemoteAddress
		report.Caps = peer.Caps
		report.EthProtocolPresent = gethclient.HasEthCapability(peer)
		if report.EthProtocolPresent {
			managed.LastSuccess = time.Now().UTC()
			managed.FailureCount = 0
			managed.CooldownUntil = time.Time{}
		}
	} else {
		report.Error = "peer not observed in admin_peers after admin_addPeer"
		managed.FailureCount++
		managed.CooldownUntil = backoff(managed.FailureCount)
	}
	return report
}

func (r *Runner) peerReport(ctx context.Context, report oracle.PeerReportRequest) error {
	return r.postJSON(ctx, "/v1/peer-report", report, nil)
}

func (r *Runner) postJSON(ctx context.Context, path string, body any, out any) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.cfg.Agent.OracleURL, "/")+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("%s failed: %s %s", path, res.Status, string(b))
	}
	if out != nil {
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

func (r *Runner) jittered(base time.Duration) time.Duration {
	j := r.cfg.Agent.JitterPercent
	if j <= 0 {
		return base
	}
	delta := int64(base) * int64(j) / 100
	return base - time.Duration(delta) + time.Duration(rand.Int63n(delta*2+1))
}

func backoff(failures int) time.Time {
	if failures < 1 {
		failures = 1
	}
	seconds := 1 << min(failures, 8)
	return time.Now().Add(time.Duration(seconds) * time.Second)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
