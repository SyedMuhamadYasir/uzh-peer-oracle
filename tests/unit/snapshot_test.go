package unit

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	uzhcrypto "github.com/uzh/uzh-peer-oracle/internal/crypto"
	"github.com/uzh/uzh-peer-oracle/internal/snapshot"
)

func TestSnapshotSignAndVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := snapshot.Create(snapshot.Input{
		NetworkID:       "702",
		ChainID:         "0x2be",
		Zone:            "public",
		Zones:           []string{"public"},
		PrivateKey:      priv,
		PublicKeyHex:     uzhcrypto.PublicKeyHex(pub),
		PublishPublicKey: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.Verify("", raw); err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-2] = 'x'
	if err := snapshot.Verify("", raw); err == nil {
		t.Fatal("expected tampered snapshot to fail verification")
	}
}
