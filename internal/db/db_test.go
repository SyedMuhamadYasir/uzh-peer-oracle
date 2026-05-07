package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/uzh/uzh-peer-oracle/internal/config"
	"github.com/uzh/uzh-peer-oracle/internal/oracle"
)

func TestSaveWireProbeResult(t *testing.T) {
	cfg := config.Defaults()
	cfg.Storage.SQLitePath = filepath.Join(t.TempDir(), "oracle.db")
	store, err := Open(context.Background(), cfg.Storage)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	in := oracle.WireProbeResult{
		NodeID:       "node-1",
		Enode:        "enode://abc@127.0.0.1:30308",
		CheckedAt:    time.Now().UTC(),
		ParseOK:      true,
		ParseStatus:  "ok",
		TCPOK:        true,
		TCPLatencyMS: 7,
		TCPStatus:    "ok",
	}
	if err := store.SaveWireProbeResult(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	got, err := store.LatestWireProbeResult(context.Background(), "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.TCPOK || got.TCPLatencyMS != 7 {
		t.Fatalf("unexpected wire probe result: %+v", got)
	}
}
