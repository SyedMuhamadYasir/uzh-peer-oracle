package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type State struct {
	ManagedPeers map[string]*ManagedPeer `json:"managed_peers"`
	LastSnapshot  []byte                  `json:"last_snapshot,omitempty"`
}

type ManagedPeer struct {
	NodeID        string    `json:"node_id"`
	Enode         string    `json:"enode"`
	LastAttempt   time.Time `json:"last_attempt"`
	LastSuccess   time.Time `json:"last_success"`
	FailureCount  int       `json:"failure_count"`
	CooldownUntil time.Time `json:"cooldown_until"`
}

func LoadState(path string) (*State, error) {
	st := &State{ManagedPeers: map[string]*ManagedPeer{}}
	if path == "" {
		return st, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, err
	}
	if st.ManagedPeers == nil {
		st.ManagedPeers = map[string]*ManagedPeer{}
	}
	return st, nil
}

func (s *State) Save(path string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
