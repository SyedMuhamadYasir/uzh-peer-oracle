package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	dialect string
}

func Open(ctx context.Context, cfg config.StorageConfig) (*Store, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	var dsn string
	var sqlDriver string
	switch driver {
	case "sqlite", "sqlite3":
		if cfg.SQLitePath == "" {
			return nil, fmt.Errorf("sqlite_path is required")
		}
		if err := os.MkdirAll(filepath.Dir(cfg.SQLitePath), 0o755); err != nil {
			return nil, err
		}
		sqlDriver = "sqlite"
		dsn = cfg.SQLitePath
		driver = "sqlite"
	case "postgres", "postgresql":
		return nil, fmt.Errorf("postgres storage is future work; Milestone 1 supports sqlite")
	default:
		return nil, fmt.Errorf("unsupported storage driver %q", cfg.Driver)
	}
	conn, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	s := &Store{db: conn, dialect: driver}
	if driver == "sqlite" {
		for _, pragma := range []string{
			"PRAGMA journal_mode=WAL",
			"PRAGMA synchronous=NORMAL",
			"PRAGMA busy_timeout=5000",
			"PRAGMA foreign_keys=ON",
		} {
			if _, err := conn.ExecContext(ctx, pragma); err != nil {
				_ = conn.Close()
				return nil, err
			}
		}
	}
	if err := s.migrate(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS nodes (
			node_id TEXT PRIMARY KEY,
			node_name TEXT,
			role TEXT,
			enode TEXT,
			enr TEXT,
			ip TEXT,
			tcp_port INTEGER,
			udp_port INTEGER,
			zones_json TEXT NOT NULL,
			zone_hints_json TEXT NOT NULL,
			network_id TEXT,
			chain_id TEXT,
			genesis_hash TEXT,
			client_version TEXT,
			block_number INTEGER,
			peer_count INTEGER,
			current_peer_ids_json TEXT NOT NULL,
			agent_version TEXT,
			status TEXT,
			score INTEGER,
			banned BOOLEAN,
			is_seed BOOLEAN,
			source TEXT,
			first_seen TEXT,
			last_seen TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_status_last_seen ON nodes(status, last_seen)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_network_chain ON nodes(network_id, chain_id)`,
		`CREATE TABLE IF NOT EXISTS reachability_edges (
			from_id TEXT NOT NULL,
			from_zone TEXT NOT NULL,
			to_id TEXT NOT NULL,
			success_count INTEGER NOT NULL,
			failure_count INTEGER NOT NULL,
			last_success TEXT,
			last_failure TEXT,
			median_connect_time_ms INTEGER,
			last_error TEXT,
			confidence_score REAL NOT NULL,
			PRIMARY KEY(from_id, from_zone, to_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_to_id ON reachability_edges(to_id)`,
		`CREATE TABLE IF NOT EXISTS heartbeats (
			created_at TEXT NOT NULL,
			node_id TEXT NOT NULL,
			payload_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_heartbeats_node ON heartbeats(node_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS peer_reports (
			created_at TEXT NOT NULL,
			from_node_id TEXT NOT NULL,
			to_node_id TEXT NOT NULL,
			payload_json TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_peer_reports_from ON peer_reports(from_node_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS probe_reports (
			created_at TEXT NOT NULL,
			probe_id TEXT NOT NULL,
			zone TEXT NOT NULL,
			payload_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			snapshot_hash TEXT PRIMARY KEY,
			generated_at TEXT NOT NULL,
			zone TEXT NOT NULL,
			payload_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS audit_events (
			created_at TEXT NOT NULL,
			kind TEXT NOT NULL,
			payload_json TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tokens (
			name TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL,
			allowed_zones_json TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertNode(ctx context.Context, n *oracle.Node) error {
	now := time.Now().UTC()
	if n.FirstSeen.IsZero() {
		n.FirstSeen = now
	}
	if n.LastSeen.IsZero() {
		n.LastSeen = now
	}
	zones, _ := json.Marshal(n.Zones)
	hints, _ := json.Marshal(n.ZoneHints)
	currentPeers, _ := json.Marshal(n.CurrentPeerIDs)
	query := `INSERT INTO nodes (
		node_id,node_name,role,enode,enr,ip,tcp_port,udp_port,zones_json,zone_hints_json,
		network_id,chain_id,genesis_hash,client_version,block_number,peer_count,current_peer_ids_json,
		agent_version,status,score,banned,is_seed,source,first_seen,last_seen
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(node_id) DO UPDATE SET
		node_name=excluded.node_name,
		role=excluded.role,
		enode=CASE WHEN excluded.enode != '' THEN excluded.enode ELSE nodes.enode END,
		enr=CASE WHEN excluded.enr != '' THEN excluded.enr ELSE nodes.enr END,
		ip=CASE WHEN excluded.ip != '' THEN excluded.ip ELSE nodes.ip END,
		tcp_port=CASE WHEN excluded.tcp_port != 0 THEN excluded.tcp_port ELSE nodes.tcp_port END,
		udp_port=CASE WHEN excluded.udp_port != 0 THEN excluded.udp_port ELSE nodes.udp_port END,
		zones_json=excluded.zones_json,
		zone_hints_json=excluded.zone_hints_json,
		network_id=excluded.network_id,
		chain_id=excluded.chain_id,
		genesis_hash=excluded.genesis_hash,
		client_version=excluded.client_version,
		block_number=excluded.block_number,
		peer_count=excluded.peer_count,
		current_peer_ids_json=excluded.current_peer_ids_json,
		agent_version=excluded.agent_version,
		status=excluded.status,
		score=excluded.score,
		banned=excluded.banned,
		is_seed=nodes.is_seed OR excluded.is_seed,
		source=excluded.source,
		last_seen=excluded.last_seen`
	_, err := s.db.ExecContext(ctx, s.q(query),
		n.NodeID, n.NodeName, n.Role, n.Enode, n.ENR, n.IP, n.TCPPort, n.UDPPort, string(zones), string(hints),
		n.NetworkID, n.ChainID, n.GenesisHash, n.ClientVersion, n.BlockNumber, n.PeerCount, string(currentPeers),
		n.AgentVersion, n.Status, n.Score, n.Banned, n.IsSeed, n.Source, formatTime(n.FirstSeen), formatTime(n.LastSeen),
	)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(n)
	_, _ = s.db.ExecContext(ctx, s.q(`INSERT INTO heartbeats(created_at,node_id,payload_json) VALUES(?,?,?)`), formatTime(now), n.NodeID, string(payload))
	return nil
}

func (s *Store) GetNode(ctx context.Context, id string) (*oracle.Node, error) {
	row := s.db.QueryRowContext(ctx, s.q(`SELECT node_id,node_name,role,enode,enr,ip,tcp_port,udp_port,zones_json,zone_hints_json,
		network_id,chain_id,genesis_hash,client_version,block_number,peer_count,current_peer_ids_json,
		agent_version,status,score,banned,is_seed,source,first_seen,last_seen FROM nodes WHERE node_id=?`), id)
	return scanNode(row)
}

func (s *Store) ListNodes(ctx context.Context) ([]*oracle.Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT node_id,node_name,role,enode,enr,ip,tcp_port,udp_port,zones_json,zone_hints_json,
		network_id,chain_id,genesis_hash,client_version,block_number,peer_count,current_peer_ids_json,
		agent_version,status,score,banned,is_seed,source,first_seen,last_seen FROM nodes`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*oracle.Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) ListEdges(ctx context.Context) ([]oracle.ReachabilityEdge, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT from_id,from_zone,to_id,success_count,failure_count,last_success,last_failure,median_connect_time_ms,last_error,confidence_score FROM reachability_edges`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []oracle.ReachabilityEdge
	for rows.Next() {
		var e oracle.ReachabilityEdge
		var lastSuccess, lastFailure string
		if err := rows.Scan(&e.FromID, &e.FromZone, &e.ToID, &e.SuccessCount, &e.FailureCount, &lastSuccess, &lastFailure, &e.MedianConnectTimeMS, &e.LastError, &e.ConfidenceScore); err != nil {
			return nil, err
		}
		e.LastSuccess = parseTime(lastSuccess)
		e.LastFailure = parseTime(lastFailure)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) UpdateReachability(ctx context.Context, fromID, fromZone, toID string, success bool, latencyMS int64, lastErr string) error {
	if fromID == "" {
		fromID = "zone:" + fromZone
	}
	if fromZone == "" {
		fromZone = "unknown"
	}
	var existing oracle.ReachabilityEdge
	var lastSuccess, lastFailure string
	err := s.db.QueryRowContext(ctx, s.q(`SELECT from_id,from_zone,to_id,success_count,failure_count,last_success,last_failure,median_connect_time_ms,last_error,confidence_score
		FROM reachability_edges WHERE from_id=? AND from_zone=? AND to_id=?`), fromID, fromZone, toID).
		Scan(&existing.FromID, &existing.FromZone, &existing.ToID, &existing.SuccessCount, &existing.FailureCount, &lastSuccess, &lastFailure, &existing.MedianConnectTimeMS, &existing.LastError, &existing.ConfidenceScore)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	now := time.Now().UTC()
	if err == sql.ErrNoRows {
		existing = oracle.ReachabilityEdge{FromID: fromID, FromZone: fromZone, ToID: toID}
	}
	if success {
		existing.SuccessCount++
		existing.LastSuccess = now
		if latencyMS > 0 {
			if existing.MedianConnectTimeMS == 0 {
				existing.MedianConnectTimeMS = latencyMS
			} else {
				existing.MedianConnectTimeMS = (existing.MedianConnectTimeMS + latencyMS) / 2
			}
		}
	} else {
		existing.FailureCount++
		existing.LastFailure = now
		existing.LastError = lastErr
	}
	existing.ConfidenceScore = confidence(existing.SuccessCount, existing.FailureCount)
	query := `INSERT INTO reachability_edges(from_id,from_zone,to_id,success_count,failure_count,last_success,last_failure,median_connect_time_ms,last_error,confidence_score)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(from_id,from_zone,to_id) DO UPDATE SET
		success_count=excluded.success_count,
		failure_count=excluded.failure_count,
		last_success=excluded.last_success,
		last_failure=excluded.last_failure,
		median_connect_time_ms=excluded.median_connect_time_ms,
		last_error=excluded.last_error,
		confidence_score=excluded.confidence_score`
	_, err = s.db.ExecContext(ctx, s.q(query), existing.FromID, existing.FromZone, existing.ToID, existing.SuccessCount, existing.FailureCount,
		formatTime(existing.LastSuccess), formatTime(existing.LastFailure), existing.MedianConnectTimeMS, existing.LastError, existing.ConfidenceScore)
	return err
}

func (s *Store) RecordPeerReport(ctx context.Context, report oracle.PeerReportRequest) error {
	payload, _ := json.Marshal(report)
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO peer_reports(created_at,from_node_id,to_node_id,payload_json) VALUES(?,?,?,?)`),
		formatTime(time.Now().UTC()), report.FromNodeID, report.ToNodeID, string(payload))
	return err
}

func (s *Store) RecordProbeReport(ctx context.Context, report oracle.ProbeReportRequest) error {
	payload, _ := json.Marshal(report)
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO probe_reports(created_at,probe_id,zone,payload_json) VALUES(?,?,?,?)`),
		formatTime(time.Now().UTC()), report.ProbeID, report.Zone, string(payload))
	return err
}

func (s *Store) AddAudit(ctx context.Context, kind string, payload any) {
	b, _ := json.Marshal(payload)
	_, _ = s.db.ExecContext(ctx, s.q(`INSERT INTO audit_events(created_at,kind,payload_json) VALUES(?,?,?)`), formatTime(time.Now().UTC()), kind, string(b))
}

func (s *Store) SaveSnapshot(ctx context.Context, hash, zone string, payload any) error {
	b, _ := json.Marshal(payload)
	_, err := s.db.ExecContext(ctx, s.q(`INSERT INTO snapshots(snapshot_hash,generated_at,zone,payload_json) VALUES(?,?,?,?)
		ON CONFLICT(snapshot_hash) DO UPDATE SET payload_json=excluded.payload_json`), hash, formatTime(time.Now().UTC()), zone, string(b))
	return err
}

func (s *Store) LatestSnapshotJSON(ctx context.Context, zone string) ([]byte, error) {
	row := s.db.QueryRowContext(ctx, s.q(`SELECT payload_json FROM snapshots WHERE zone=? ORDER BY generated_at DESC LIMIT 1`), zone)
	var payload string
	if err := row.Scan(&payload); err != nil {
		return nil, err
	}
	return []byte(payload), nil
}

func (s *Store) q(query string) string {
	if s.dialect != "postgres" {
		return query
	}
	var b strings.Builder
	arg := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			b.WriteString("$")
			b.WriteString(strconv.Itoa(arg))
			arg++
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanNode(row scanner) (*oracle.Node, error) {
	n := &oracle.Node{}
	var zonesJSON, hintsJSON, peersJSON, firstSeen, lastSeen string
	if err := row.Scan(&n.NodeID, &n.NodeName, &n.Role, &n.Enode, &n.ENR, &n.IP, &n.TCPPort, &n.UDPPort, &zonesJSON, &hintsJSON,
		&n.NetworkID, &n.ChainID, &n.GenesisHash, &n.ClientVersion, &n.BlockNumber, &n.PeerCount, &peersJSON,
		&n.AgentVersion, &n.Status, &n.Score, &n.Banned, &n.IsSeed, &n.Source, &firstSeen, &lastSeen); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(zonesJSON), &n.Zones)
	_ = json.Unmarshal([]byte(hintsJSON), &n.ZoneHints)
	_ = json.Unmarshal([]byte(peersJSON), &n.CurrentPeerIDs)
	n.FirstSeen = parseTime(firstSeen)
	n.LastSeen = parseTime(lastSeen)
	return n, nil
}

func confidence(success, failure int) float64 {
	total := success + failure
	return float64(success+1) / float64(total+2)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}
