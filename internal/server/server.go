package server

import (
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	uzhcrypto "github.com/uzh/uzh-peer-oracle/internal/crypto"
	"github.com/uzh/uzh-peer-oracle/internal/db"
	"github.com/uzh/uzh-peer-oracle/internal/enodeutil"
	"github.com/uzh/uzh-peer-oracle/internal/log"
	"github.com/uzh/uzh-peer-oracle/internal/matcher"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
	"github.com/uzh/uzh-peer-oracle/internal/seed"
	"github.com/uzh/uzh-peer-oracle/internal/snapshot"
)

type App struct {
	cfg       *config.Config
	store     *db.Store
	logger    *log.Logger
	keypair   *uzhcrypto.Keypair
	startedAt time.Time
	prevHash  string
	mu        sync.Mutex
	metrics   Metrics
	limiters  map[string]*limiter
}

type Metrics struct {
	Heartbeats       int64
	PeersRequests    int64
	PeerReports      int64
	ProbeReports     int64
	Rejected         int64
	WrongNetwork     int64
	WrongChain       int64
	LastSnapshotHash string
}

type AuthContext struct {
	Name         string
	AllowedZones []string
	ExplicitZones bool
	Role         string
}

func New(ctx context.Context, cfg *config.Config, logger *log.Logger) (*App, error) {
	store, err := db.Open(ctx, cfg.Storage)
	if err != nil {
		return nil, err
	}
	var kp *uzhcrypto.Keypair
	if cfg.Snapshot.Enabled {
		kp, err = uzhcrypto.LoadOrCreate(cfg.Snapshot.PrivateKeyPath)
		if err != nil {
			_ = store.Close()
			return nil, err
		}
	}
	return &App{
		cfg:       cfg,
		store:     store,
		logger:    logger,
		keypair:   kp,
		startedAt: time.Now().UTC(),
		limiters:  map[string]*limiter{},
	}, nil
}

func (a *App) Close() error {
	return a.store.Close()
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/v1/heartbeat", a.withAuth(a.handleHeartbeat))
	mux.HandleFunc("/v1/peers", a.withAuth(a.handlePeers))
	mux.HandleFunc("/v1/debug/nodes", a.withAuth(a.handleDebugNodes))
	return http.TimeoutHandler(http.MaxBytesHandler(a.rateLimit(mux), 1<<20), 15*time.Second, `{"error":"request timeout"}`)
}

func (a *App) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              a.cfg.Server.Listen,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("oracle server listening", map[string]any{"addr": a.cfg.Server.Listen})
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (a *App) IngestSeedFile(ctx context.Context, path string) (*seed.Result, error) {
	res, err := seed.ParseFile(path, a.cfg)
	if err != nil {
		return nil, err
	}
	for _, p := range res.Peers {
		if err := a.store.UpsertNode(ctx, p); err != nil {
			return nil, err
		}
	}
	a.store.AddAudit(ctx, "seed_ingest", map[string]any{"path": path, "peers": len(res.Peers), "errors": res.Errors})
	a.logger.Info("seed file ingested", map[string]any{"path": path, "peers": len(res.Peers), "errors": len(res.Errors)})
	return res, nil
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"started_at": a.startedAt,
		"version":    oracle.AgentVersion,
	})
}

func (a *App) handleHeartbeat(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var hb oracle.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		writeError(w, http.StatusBadRequest, "invalid heartbeat JSON")
		return
	}
	resp := a.verifyHeartbeat(r.Context(), r, &hb, auth)
	if !resp.Accepted {
		a.metrics.Rejected++
		a.store.AddAudit(r.Context(), "heartbeat_rejected", map[string]any{"node_id": hb.NodeID, "reason": resp.RejectReason})
		writeJSON(w, http.StatusOK, resp)
		return
	}
	hb.Zones = resp.EffectiveZones
	hb.Score = resp.Score
	hb.Status = resp.Status
	hb.LastSeen = time.Now().UTC()
	if hb.Role == "" {
		hb.Role = "normal"
	}
	if hb.AgentVersion == "" {
		hb.AgentVersion = oracle.AgentVersion
	}
	if err := a.store.UpsertNode(r.Context(), &hb); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.metrics.Heartbeats++
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) verifyHeartbeat(ctx context.Context, r *http.Request, hb *oracle.HeartbeatRequest, auth AuthContext) oracle.HeartbeatResponse {
	var warnings []string
	reject := func(reason string) oracle.HeartbeatResponse {
		return oracle.HeartbeatResponse{Accepted: false, Status: "rejected", Warnings: warnings, RejectReason: &reason}
	}
	if hb.NodeID == "" {
		if hb.Enode == "" {
			return reject("missing node_id and enode")
		}
		parsed, err := enodeutil.Parse(hb.Enode)
		if err != nil {
			return reject("invalid enode")
		}
		hb.NodeID = parsed.NodeID
	}
	if hb.Enode != "" {
		if parsed, err := enodeutil.Parse(hb.Enode); err == nil {
			if hb.IP == "" {
				hb.IP = parsed.IP
			}
			if hb.TCPPort == 0 {
				hb.TCPPort = parsed.TCPPort
			}
			if hb.UDPPort == 0 {
				hb.UDPPort = parsed.UDPPort
			}
		} else {
			warnings = append(warnings, "invalid enode format")
		}
	}
	if a.cfg.Verification.RequireCorrectNetworkID && a.cfg.Network.ExpectedNetworkID != "" && hb.NetworkID != "" && hb.NetworkID != a.cfg.Network.ExpectedNetworkID {
		a.metrics.WrongNetwork++
		return reject("wrong network_id")
	}
	if a.cfg.Verification.RequireCorrectChainID && a.cfg.Network.ExpectedChainID != "" && hb.ChainID != "" && !strings.EqualFold(hb.ChainID, a.cfg.Network.ExpectedChainID) {
		a.metrics.WrongChain++
		return reject("wrong chain_id")
	}
	if a.cfg.Verification.RequireGenesisHashIfConfigured && a.cfg.Network.ExpectedGenesisHash != "" && hb.GenesisHash != "" && !strings.EqualFold(hb.GenesisHash, a.cfg.Network.ExpectedGenesisHash) {
		return reject("wrong genesis_hash")
	}
	if hb.NetworkID == "" {
		hb.NetworkID = a.cfg.Network.ExpectedNetworkID
		warnings = append(warnings, "network_id missing; accepted as degraded")
	}
	if hb.ChainID == "" {
		hb.ChainID = a.cfg.Network.ExpectedChainID
		warnings = append(warnings, "chain_id missing; accepted as degraded")
	}
	effective := a.effectiveZones(r, hb, auth)
	if len(effective) == 0 {
		effective = []string{a.cfg.Zones.DefaultPublicZone}
	}
	if contains(effective, a.cfg.Zones.DefaultPublicZone) && hb.IP != "" && enodeutil.IsPrivateOrLoopback(hb.IP) {
		warnings = append(warnings, "public zone requested but enode/IP is private or loopback")
		effective = remove(effective, a.cfg.Zones.DefaultPublicZone)
		if len(effective) == 0 {
			effective = []string{"unknown"}
		}
	}
	status := "verified"
	if len(warnings) > 0 {
		status = "degraded"
	}
	score := 4
	if hb.Role == "hub" {
		score += 10
	}
	if hb.PeerCount > 0 {
		score += min(hb.PeerCount, 8)
	}
	if hb.BlockNumber > 0 {
		score += 2
	}
	if hb.Enode != "" {
		score += 2
	}
	return oracle.HeartbeatResponse{Accepted: true, Status: status, Score: score, EffectiveZones: effective, Warnings: warnings}
}

func (a *App) handlePeers(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	nodeID := r.URL.Query().Get("node_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var requester *oracle.Node
	if nodeID != "" {
		if n, err := a.store.GetNode(r.Context(), nodeID); err == nil {
			requester = n
		}
	}
	effective := []string{a.cfg.Zones.DefaultPublicZone}
	if requester != nil {
		effective = requester.Zones
	}
	if len(auth.AllowedZones) > 0 {
		effective = intersectOrPublic(effective, auth.AllowedZones, a.cfg.Zones.DefaultPublicZone)
	}
	nodes, err := a.store.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	edges, err := a.store.ListEdges(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	recs := matcher.Select(matcher.Request{Requester: requester, EffectiveZones: effective, Limit: limit, Now: time.Now().UTC()}, nodes, edges, a.cfg)
	snap, _, err := a.createSnapshot(r.Context(), "response", effective, recs, edges)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.metrics.PeersRequests++
	writeJSON(w, http.StatusOK, oracle.PeersResponse{SnapshotHash: snap.SnapshotHash, Signature: snap.Signature, Peers: recs})
}

func (a *App) handlePeerReport(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var report oracle.PeerReportRequest
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		writeError(w, http.StatusBadRequest, "invalid peer report JSON")
		return
	}
	if report.AttemptedAt.IsZero() {
		report.AttemptedAt = time.Now().UTC()
	}
	success := report.AdminAddPeerResult && report.ObservedInAdminPeers && report.EthProtocolPresent
	if err := a.store.RecordPeerReport(r.Context(), report); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	fromZones := []string{"unknown"}
	if from, err := a.store.GetNode(r.Context(), report.FromNodeID); err == nil && len(from.Zones) > 0 {
		fromZones = from.Zones
	}
	for _, z := range fromZones {
		_ = a.store.UpdateReachability(r.Context(), report.FromNodeID, z, report.ToNodeID, success, 0, report.Error)
	}
	a.metrics.PeerReports++
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true})
}

func (a *App) handleProbeReport(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var report oracle.ProbeReportRequest
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		writeError(w, http.StatusBadRequest, "invalid probe report JSON")
		return
	}
	if report.Zone == "" {
		writeError(w, http.StatusBadRequest, "probe zone required")
		return
	}
	if len(auth.AllowedZones) > 0 && !contains(auth.AllowedZones, report.Zone) {
		writeError(w, http.StatusForbidden, "token is not allowed to report this zone")
		return
	}
	if err := a.store.RecordProbeReport(r.Context(), report); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, obs := range report.Observations {
		if obs.NodeID == "" && obs.Enode != "" {
			if parsed, err := enodeutil.Parse(obs.Enode); err == nil {
				obs.NodeID = parsed.NodeID
			}
		}
		if obs.NodeID == "" {
			continue
		}
		success := obs.TCPReachable || obs.DevP2PV4OK || obs.DevP2PV5OK
		_ = a.store.UpdateReachability(r.Context(), "probe:"+report.ProbeID, report.Zone, obs.NodeID, success, obs.LatencyMS, obs.Error)
		if success {
			if n, err := a.store.GetNode(r.Context(), obs.NodeID); err == nil && !contains(n.Zones, report.Zone) {
				n.Zones = append(n.Zones, report.Zone)
				_ = a.store.UpsertNode(r.Context(), n)
			}
		}
	}
	a.metrics.ProbeReports++
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true})
}

func (a *App) handleBootnodeEnodes(w http.ResponseWriter, r *http.Request) {
	nodes := a.publicVerifiedNodes(r.Context())
	var enodes []string
	for _, n := range nodes {
		if n.Enode != "" {
			enodes = append(enodes, n.Enode)
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(strings.Join(enodes, "\n") + "\n"))
}

func (a *App) handleBootnodeENRs(w http.ResponseWriter, r *http.Request) {
	nodes := a.publicVerifiedNodes(r.Context())
	var enrs []string
	for _, n := range nodes {
		if n.ENR != "" {
			enrs = append(enrs, n.ENR)
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(strings.Join(enrs, "\n") + "\n"))
}

func (a *App) handleBootnodeTOML(w http.ResponseWriter, r *http.Request) {
	nodes := a.publicVerifiedNodes(r.Context())
	var enodes, enrs []string
	for _, n := range nodes {
		if n.Enode != "" {
			enodes = append(enodes, fmt.Sprintf("  %q", n.Enode))
		}
		if n.ENR != "" {
			enrs = append(enrs, fmt.Sprintf("  %q", n.ENR))
		}
	}
	body := "[Node.P2P]\nBootstrapNodes = [\n" + strings.Join(enodes, ",\n") + "\n]\n\nBootstrapNodesV5 = [\n" + strings.Join(enrs, ",\n") + "\n]\n"
	w.Header().Set("Content-Type", "application/toml; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (a *App) handleDNSNodes(w http.ResponseWriter, r *http.Request) {
	nodes := a.publicVerifiedNodes(r.Context())
	type dnsNode struct {
		NodeID string   `json:"node_id"`
		Enode  string   `json:"enode,omitempty"`
		ENR    string   `json:"enr,omitempty"`
		Zones  []string `json:"zones"`
	}
	var out []dnsNode
	for _, n := range nodes {
		out = append(out, dnsNode{NodeID: n.NodeID, Enode: n.Enode, ENR: n.ENR, Zones: n.Zones})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"format": "curated-devp2p-nodeset",
		"note":   "Use docs/dns-discovery.md to convert this curated public set into signed DNS discovery material.",
		"nodes":  out,
	})
}

func (a *App) handleSnapshotLatest(w http.ResponseWriter, r *http.Request) {
	a.serveSnapshot(w, r, "latest", []string{a.cfg.Zones.DefaultPublicZone})
}

func (a *App) handleSnapshotPublic(w http.ResponseWriter, r *http.Request) {
	a.serveSnapshot(w, r, "public", []string{a.cfg.Zones.DefaultPublicZone})
}

func (a *App) handleSnapshotZone(w http.ResponseWriter, r *http.Request) {
	zone := strings.TrimPrefix(r.URL.Path, "/v1/snapshot/zone/")
	if zone == "" {
		writeError(w, http.StatusBadRequest, "missing zone")
		return
	}
	a.serveSnapshot(w, r, zone, []string{zone})
}

func (a *App) serveSnapshot(w http.ResponseWriter, r *http.Request, zone string, zones []string) {
	nodes, err := a.store.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	edges, err := a.store.ListEdges(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	recs := matcher.Select(matcher.Request{EffectiveZones: zones, Limit: a.cfg.Selection.MaxPeersReturned, Now: time.Now().UTC()}, nodes, edges, a.cfg)
	_, raw, err := a.createSnapshot(r.Context(), zone, zones, recs, edges)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

func (a *App) handleDebugNodes(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	nodes, err := a.store.ListNodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, nodes)
}

func (a *App) handleDebugGraph(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	nodes, _ := a.store.ListNodes(r.Context())
	edges, _ := a.store.ListEdges(r.Context())
	writeJSON(w, http.StatusOK, oracle.DebugGraph{Nodes: nodes, Edges: edges})
}

func (a *App) handleDebugZones(w http.ResponseWriter, r *http.Request, auth AuthContext) {
	writeJSON(w, http.StatusOK, map[string]any{"zones": a.cfg.AllZones(), "university_cidrs": a.cfg.Zones.UniversityCIDRs})
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	m := a.metrics
	a.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	lines := []string{
		fmt.Sprintf("uzh_peer_oracle_uptime_seconds %d", int64(time.Since(a.startedAt).Seconds())),
		fmt.Sprintf("uzh_peer_oracle_heartbeats_total %d", m.Heartbeats),
		fmt.Sprintf("uzh_peer_oracle_peer_requests_total %d", m.PeersRequests),
		fmt.Sprintf("uzh_peer_oracle_peer_reports_total %d", m.PeerReports),
		fmt.Sprintf("uzh_peer_oracle_probe_reports_total %d", m.ProbeReports),
		fmt.Sprintf("uzh_peer_oracle_rejected_heartbeats_total %d", m.Rejected),
		fmt.Sprintf("uzh_peer_oracle_wrong_network_total %d", m.WrongNetwork),
		fmt.Sprintf("uzh_peer_oracle_wrong_chain_total %d", m.WrongChain),
	}
	_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/dashboard" {
		http.NotFound(w, r)
		return
	}
	nodes, _ := a.store.ListNodes(r.Context())
	edges, _ := a.store.ListEdges(r.Context())
	now := time.Now().UTC()
	var verified, public, private, stale, wrongChain int
	var topHubs []*oracle.Node
	var heights []uint64
	for _, n := range nodes {
		if n.Status == "verified" {
			verified++
		}
		if n.HasZone(a.cfg.Zones.DefaultPublicZone) {
			public++
		}
		if !n.HasZone(a.cfg.Zones.DefaultPublicZone) {
			private++
		}
		if !n.LastSeen.IsZero() && now.Sub(n.LastSeen) > time.Duration(a.cfg.Verification.StaleAfterSeconds)*time.Second {
			stale++
		}
		if n.ChainID != "" && a.cfg.Network.ExpectedChainID != "" && !strings.EqualFold(n.ChainID, a.cfg.Network.ExpectedChainID) {
			wrongChain++
		}
		if n.Role == "hub" {
			topHubs = append(topHubs, n)
		}
		if n.BlockNumber > 0 {
			heights = append(heights, n.BlockNumber)
		}
	}
	sort.Slice(topHubs, func(i, j int) bool { return topHubs[i].Score > topHubs[j].Score })
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	heightSpread := "n/a"
	if len(heights) > 0 {
		heightSpread = fmt.Sprintf("%d - %d", heights[0], heights[len(heights)-1])
	}
	a.mu.Lock()
	snap := a.metrics.LastSnapshotHash
	a.mu.Unlock()
	var hubRows []string
	for i, h := range topHubs {
		if i >= 8 {
			break
		}
		hubRows = append(hubRows, fmt.Sprintf("<tr><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>",
			html.EscapeString(h.NodeName), shortID(h.NodeID), h.Score, html.EscapeString(strings.Join(h.Zones, ","))))
	}
	body := fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>UZH Peer Oracle</title>
<style>
body{font-family:system-ui,-apple-system,Segoe UI,sans-serif;margin:0;background:#f7f7f4;color:#1b1b1b}
header{background:#0f2f2e;color:white;padding:22px 32px}
main{padding:24px 32px;max-width:1180px;margin:auto}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:12px}
.metric{background:white;border:1px solid #ddd;border-radius:6px;padding:14px}
.metric b{display:block;font-size:28px}
table{border-collapse:collapse;width:100%%;background:white;border:1px solid #ddd}
td,th{border-bottom:1px solid #eee;padding:8px;text-align:left;font-size:14px}
.section{margin-top:24px}
</style></head><body><header><h1>UZH Peer Oracle</h1><div>Topology-aware Geth Treffpunkt for network_id 702 / chain_id 0x2be</div></header><main>
<div class="grid">
<div class="metric"><span>Total nodes</span><b>%d</b></div>
<div class="metric"><span>Verified</span><b>%d</b></div>
<div class="metric"><span>Public</span><b>%d</b></div>
<div class="metric"><span>VPN/private</span><b>%d</b></div>
<div class="metric"><span>Stale</span><b>%d</b></div>
<div class="metric"><span>Edges</span><b>%d</b></div>
<div class="metric"><span>Block spread</span><b>%s</b></div>
<div class="metric"><span>Wrong-chain rejects</span><b>%d</b></div>
</div>
<div class="section"><h2>Signed Snapshot</h2><p><code>%s</code></p></div>
<div class="section"><h2>Top Hubs</h2><table><tr><th>Name</th><th>Node</th><th>Score</th><th>Zones</th></tr>%s</table></div>
<div class="section"><h2>Reachability Graph</h2><p>%d directed observations across zone and node sources.</p></div>
</main></body></html>`, len(nodes), verified, public, private, stale, len(edges), heightSpread, wrongChain, html.EscapeString(snap), strings.Join(hubRows, ""), len(edges))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

func (a *App) createSnapshot(ctx context.Context, zone string, zones []string, recs []oracle.PeerRecommendation, edges []oracle.ReachabilityEdge) (*snapshot.Snapshot, []byte, error) {
	if !a.cfg.Snapshot.Enabled || a.keypair == nil {
		return &snapshot.Snapshot{}, []byte("{}"), nil
	}
	a.mu.Lock()
	prev := a.prevHash
	a.mu.Unlock()
	snap, raw, err := snapshot.Create(snapshot.Input{
		NetworkID:            a.cfg.Network.ExpectedNetworkID,
		ChainID:              a.cfg.Network.ExpectedChainID,
		GenesisHash:          a.cfg.Network.ExpectedGenesisHash,
		Zone:                 zone,
		Zones:                zones,
		Peers:                recs,
		ReachabilitySummary:  edges,
		PreviousSnapshotHash: prev,
		PrivateKey:           a.keypair.Private,
		PublicKeyHex:          uzhcrypto.PublicKeyHex(a.keypair.Public),
		PublishPublicKey:      a.cfg.Snapshot.PublishPublicKey,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := a.store.SaveSnapshot(ctx, snap.SnapshotHash, zone, snap); err != nil {
		return nil, nil, err
	}
	a.mu.Lock()
	a.prevHash = snap.SnapshotHash
	a.metrics.LastSnapshotHash = snap.SnapshotHash
	a.mu.Unlock()
	return snap, raw, nil
}

func (a *App) publicVerifiedNodes(ctx context.Context) []*oracle.Node {
	nodes, err := a.store.ListNodes(ctx)
	if err != nil {
		return nil
	}
	var out []*oracle.Node
	now := time.Now().UTC()
	for _, n := range nodes {
		if n.Banned || n.Enode == "" || !n.HasZone(a.cfg.Zones.DefaultPublicZone) {
			continue
		}
		if n.IP != "" && !enodeutil.IsPublicIP(n.IP) {
			continue
		}
		if !n.LastSeen.IsZero() && now.Sub(n.LastSeen) > time.Duration(a.cfg.Verification.DeadAfterSeconds)*time.Second {
			continue
		}
		out = append(out, n)
	}
	return out
}

func (a *App) withAuth(next func(http.ResponseWriter, *http.Request, AuthContext)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth, ok := a.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		next(w, r, auth)
	}
}

func (a *App) authenticate(r *http.Request) (AuthContext, bool) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return AuthContext{}, false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if token == "" {
		return AuthContext{}, false
	}
	for _, t := range a.cfg.Auth.Tokens {
		expected := t.Token
		if expected == "" && t.TokenEnv != "" {
			expected = strings.TrimSpace(getenv(t.TokenEnv))
		}
		if expected != "" && hmac.Equal([]byte(token), []byte(expected)) {
			return AuthContext{Name: t.Name, AllowedZones: t.AllowedZones, ExplicitZones: len(t.AllowedZones) > 0, Role: t.Role}, true
		}
	}
	if shared := a.cfg.SharedToken(); shared != "" && hmac.Equal([]byte(token), []byte(shared)) {
		return AuthContext{Name: "shared", Role: "shared"}, true
	}
	return AuthContext{}, false
}

func (a *App) effectiveZones(r *http.Request, hb *oracle.HeartbeatRequest, auth AuthContext) []string {
	sourceIP := clientIP(r)
	sourceAllowedPrivate := a.sourceInUniversityCIDR(sourceIP) || isPrivateSource(sourceIP)
	allowedByToken := set(auth.AllowedZones)
	publicZone := a.cfg.Zones.DefaultPublicZone
	out := map[string]bool{}
	if hb.IP != "" && enodeutil.IsPublicIP(hb.IP) {
		out[publicZone] = true
	}
	for _, z := range append(hb.Zones, hb.ZoneHints...) {
		z = strings.TrimSpace(z)
		if z == "" {
			continue
		}
		if z == publicZone {
			out[z] = true
			continue
		}
		if auth.ExplicitZones && allowedByToken[z] {
			out[z] = true
			continue
		}
		if !auth.ExplicitZones && sourceAllowedPrivate && isConfiguredPrivateZone(z, a.cfg) {
			out[z] = true
		}
	}
	if sourceAllowedPrivate {
		for _, z := range a.cfg.Zones.PrivateZones {
			if contains(hb.ZoneHints, z) || contains(hb.Zones, z) {
				out[z] = true
			}
		}
	}
	if len(out) == 0 {
		out[publicZone] = true
	}
	var zones []string
	for z := range out {
		zones = append(zones, z)
	}
	sort.Strings(zones)
	return zones
}

func (a *App) sourceInUniversityCIDR(ipText string) bool {
	ip := net.ParseIP(ipText)
	if ip == nil {
		return false
	}
	for _, cidr := range a.cfg.Zones.UniversityCIDRs {
		_, n, err := net.ParseCIDR(cidr)
		if err == nil && n.Contains(ip) {
			return true
		}
	}
	return false
}

func (a *App) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r)
		a.mu.Lock()
		l := a.limiters[key]
		if l == nil {
			l = &limiter{tokens: 60, last: time.Now()}
			a.limiters[key] = l
		}
		ok := l.allow()
		a.mu.Unlock()
		if !ok {
			writeError(w, http.StatusTooManyRequests, "rate limited")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type limiter struct {
	tokens float64
	last   time.Time
}

func (l *limiter) allow() bool {
	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	l.last = now
	l.tokens += elapsed * 20
	if l.tokens > 60 {
		l.tokens = 60
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func getenv(k string) string {
	return strings.TrimSpace(os.Getenv(k))
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func isPrivateSource(ipText string) bool {
	ip := net.ParseIP(ipText)
	return ip != nil && (ip.IsPrivate() || ip.IsLoopback())
}

func isConfiguredPrivateZone(zone string, cfg *config.Config) bool {
	for _, z := range cfg.Zones.PrivateZones {
		if z == zone {
			return true
		}
	}
	for _, z := range cfg.Zones.CustomZones {
		if z == zone {
			return true
		}
	}
	return false
}

func set(in []string) map[string]bool {
	out := map[string]bool{}
	for _, x := range in {
		if strings.TrimSpace(x) != "" {
			out[x] = true
		}
	}
	return out
}

func contains(in []string, x string) bool {
	for _, v := range in {
		if v == x {
			return true
		}
	}
	return false
}

func remove(in []string, x string) []string {
	var out []string
	for _, v := range in {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

func intersectOrPublic(aZones, bZones []string, public string) []string {
	allowed := set(bZones)
	var out []string
	for _, z := range aZones {
		if allowed[z] {
			out = append(out, z)
		}
	}
	if len(out) == 0 {
		out = []string{public}
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func shortID(id string) string {
	if len(id) <= 12 {
		return html.EscapeString(id)
	}
	return html.EscapeString(id[:12])
}
