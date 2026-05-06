package snapshot

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/crypto"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

type Snapshot struct {
	SnapshotVersion     int                         `json:"snapshot_version"`
	GeneratedAt         time.Time                   `json:"generated_at"`
	NetworkID           string                      `json:"network_id"`
	ChainID             string                      `json:"chain_id"`
	GenesisHash         string                      `json:"genesis_hash"`
	Zone                string                      `json:"zone"`
	Peers               []oracle.PeerRecommendation `json:"peers"`
	Zones               []string                    `json:"zones"`
	ReachabilitySummary []oracle.ReachabilityEdge   `json:"reachability_summaries"`
	PreviousSnapshotHash string                     `json:"previous_snapshot_hash"`
	SnapshotHash        string                      `json:"snapshot_hash"`
	Signature           string                      `json:"signature"`
	PublicKey           string                      `json:"public_key,omitempty"`
}

type Input struct {
	NetworkID            string
	ChainID              string
	GenesisHash          string
	Zone                 string
	Zones                []string
	Peers                []oracle.PeerRecommendation
	ReachabilitySummary  []oracle.ReachabilityEdge
	PreviousSnapshotHash string
	PrivateKey           ed25519.PrivateKey
	PublicKeyHex          string
	PublishPublicKey      bool
}

func Create(in Input) (*Snapshot, []byte, error) {
	s := &Snapshot{
		SnapshotVersion:      1,
		GeneratedAt:          time.Now().UTC(),
		NetworkID:            in.NetworkID,
		ChainID:              in.ChainID,
		GenesisHash:          in.GenesisHash,
		Zone:                 in.Zone,
		Peers:                append([]oracle.PeerRecommendation(nil), in.Peers...),
		Zones:                append([]string(nil), in.Zones...),
		ReachabilitySummary:  append([]oracle.ReachabilityEdge(nil), in.ReachabilitySummary...),
		PreviousSnapshotHash: in.PreviousSnapshotHash,
	}
	sort.Slice(s.Peers, func(i, j int) bool { return s.Peers[i].NodeID < s.Peers[j].NodeID })
	sort.Strings(s.Zones)
	canonical, err := canonicalBytes(s)
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(canonical)
	s.SnapshotHash = "0x" + hex.EncodeToString(sum[:])
	canonical, err = canonicalBytes(s)
	if err != nil {
		return nil, nil, err
	}
	s.Signature = crypto.SignHex(in.PrivateKey, canonical)
	if in.PublishPublicKey {
		s.PublicKey = in.PublicKeyHex
	}
	final, err := json.MarshalIndent(s, "", "  ")
	return s, final, err
}

func Verify(pubHex string, raw []byte) error {
	var s Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	sig := s.Signature
	hash := s.SnapshotHash
	hashCopy := s
	hashCopy.Signature = ""
	hashCopy.PublicKey = ""
	hashCopy.SnapshotHash = ""
	hashPayload, err := json.Marshal(hashCopy)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(hashPayload)
	expectedHash := "0x" + hex.EncodeToString(sum[:])
	if hash != expectedHash {
		return fmt.Errorf("snapshot hash mismatch: got %s expected %s", hash, expectedHash)
	}
	s.Signature = ""
	if s.PublicKey != "" && pubHex == "" {
		pubHex = s.PublicKey
	}
	canonical, err := canonicalBytes(&s)
	if err != nil {
		return err
	}
	return crypto.VerifyHex(pubHex, sig, canonical)
}

func canonicalBytes(s *Snapshot) ([]byte, error) {
	cp := *s
	cp.Signature = ""
	cp.PublicKey = ""
	return json.Marshal(cp)
}
