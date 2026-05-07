package oracle

import "time"

const AgentVersion = "0.1.0"

type Node struct {
	NodeName       string    `json:"node_name"`
	Role           string    `json:"role"`
	NodeID         string    `json:"node_id"`
	Enode          string    `json:"enode,omitempty"`
	ENR            string    `json:"enr,omitempty"`
	IP             string    `json:"ip,omitempty"`
	TCPPort        int       `json:"tcp_port,omitempty"`
	UDPPort        int       `json:"udp_port,omitempty"`
	Zones          []string  `json:"zones"`
	ZoneHints      []string  `json:"zone_hints,omitempty"`
	NetworkID      string    `json:"network_id"`
	ChainID        string    `json:"chain_id"`
	GenesisHash    string    `json:"genesis_hash"`
	ClientVersion  string    `json:"client_version"`
	BlockNumber    uint64    `json:"block_number"`
	PeerCount      int       `json:"peer_count"`
	CurrentPeerIDs []string  `json:"current_peer_ids,omitempty"`
	AgentVersion   string    `json:"agent_version,omitempty"`
	Status         string    `json:"status"`
	Score          int       `json:"score"`
	Banned         bool      `json:"banned"`
	IsSeed         bool      `json:"is_seed"`
	Source         string    `json:"source,omitempty"`
	FirstSeen      time.Time `json:"first_seen,omitempty"`
	LastSeen       time.Time `json:"last_seen,omitempty"`
}

type HeartbeatRequest = Node

type HeartbeatResponse struct {
	Accepted       bool     `json:"accepted"`
	Status         string   `json:"status"`
	Score          int      `json:"score"`
	EffectiveZones []string `json:"effective_zones"`
	Warnings       []string `json:"warnings"`
	RejectReason   *string  `json:"reject_reason"`
}

type PeerRecommendation struct {
	NodeID                 string    `json:"node_id"`
	NodeName               string    `json:"node_name"`
	Role                   string    `json:"role"`
	Enode                  string    `json:"enode,omitempty"`
	ENR                    string    `json:"enr,omitempty"`
	IP                     string    `json:"ip,omitempty"`
	TCPPort                int       `json:"tcp_port,omitempty"`
	UDPPort                int       `json:"udp_port,omitempty"`
	Zones                  []string  `json:"zones"`
	Score                  int       `json:"score"`
	ReachabilityConfidence float64   `json:"reachability_confidence"`
	LastSeen               time.Time `json:"last_seen"`
}

type WireProbeResult struct {
	ID              int64     `json:"id,omitempty"`
	NodeID          string    `json:"node_id,omitempty"`
	Enode           string    `json:"enode,omitempty"`
	ENR             string    `json:"enr,omitempty"`
	IP              string    `json:"ip,omitempty"`
	TCPPort         int       `json:"tcp_port,omitempty"`
	UDPPort         int       `json:"udp_port,omitempty"`
	CheckedAt       time.Time `json:"checked_at"`
	IPClass         string    `json:"ip_class,omitempty"`
	ParseOK         bool      `json:"parse_ok"`
	ParseStatus     string    `json:"parse_status,omitempty"`
	TCPOK           bool      `json:"tcp_ok"`
	TCPLatencyMS    int64     `json:"tcp_latency_ms,omitempty"`
	TCPStatus       string    `json:"tcp_status,omitempty"`
	Discv4OK        bool      `json:"discv4_ok"`
	Discv4Status    string    `json:"discv4_status,omitempty"`
	Discv5OK        bool      `json:"discv5_ok"`
	Discv5Status    string    `json:"discv5_status,omitempty"`
	RLPxOK          bool      `json:"rlpx_ok"`
	RLPxStatus      string    `json:"rlpx_status,omitempty"`
	EthStatusOK     bool      `json:"eth_status_ok"`
	EthStatusStatus string    `json:"eth_status_status,omitempty"`
	RemoteNodeID    string    `json:"remote_node_id,omitempty"`
	Caps            []string  `json:"caps,omitempty"`
	NetworkID       string    `json:"network_id,omitempty"`
	GenesisHash     string    `json:"genesis_hash,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type PeersResponse struct {
	SnapshotHash string               `json:"snapshot_hash"`
	Signature    string               `json:"signature"`
	Peers        []PeerRecommendation `json:"peers"`
}

type PeerReportRequest struct {
	FromNodeID            string    `json:"from_node_id"`
	ToNodeID              string    `json:"to_node_id"`
	ToEnode               string    `json:"to_enode"`
	AttemptedAt           time.Time `json:"attempted_at"`
	AdminAddPeerResult    bool      `json:"admin_add_peer_result"`
	ConnectedAfterSeconds bool      `json:"connected_after_seconds"`
	ObservedInAdminPeers  bool      `json:"observed_in_admin_peers"`
	RemoteAddress         string    `json:"remote_address"`
	Caps                  []string  `json:"caps"`
	EthProtocolPresent    bool      `json:"eth_protocol_present"`
	Error                 string    `json:"error"`
}

type ProbeReportRequest struct {
	ProbeID      string             `json:"probe_id"`
	Zone         string             `json:"zone"`
	Observations []ProbeObservation `json:"observations"`
}

type ProbeObservation struct {
	NodeID       string `json:"node_id"`
	Enode        string `json:"enode"`
	TCPReachable bool   `json:"tcp_reachable"`
	UDPSeen      bool   `json:"udp_seen"`
	DevP2PV4OK   bool   `json:"devp2p_v4_ok"`
	DevP2PV5OK   bool   `json:"devp2p_v5_ok"`
	LatencyMS    int64  `json:"latency_ms"`
	Error        string `json:"error"`
}

type ReachabilityEdge struct {
	FromID              string    `json:"from_id"`
	FromZone            string    `json:"from_zone"`
	ToID                string    `json:"to_id"`
	SuccessCount        int       `json:"success_count"`
	FailureCount        int       `json:"failure_count"`
	LastSuccess         time.Time `json:"last_success,omitempty"`
	LastFailure         time.Time `json:"last_failure,omitempty"`
	MedianConnectTimeMS int64     `json:"median_connect_time_ms,omitempty"`
	LastError           string    `json:"last_error,omitempty"`
	ConfidenceScore     float64   `json:"confidence_score"`
}

type DebugGraph struct {
	Nodes []*Node            `json:"nodes"`
	Edges []ReachabilityEdge `json:"edges"`
}

func (n *Node) HasZone(zone string) bool {
	for _, z := range n.Zones {
		if z == zone {
			return true
		}
	}
	return false
}
