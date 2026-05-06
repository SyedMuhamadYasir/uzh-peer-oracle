package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Keypair struct {
	Public  ed25519.PublicKey
	Private ed25519.PrivateKey
}

func LoadOrCreate(path string) (*Keypair, error) {
	if path == "" {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		return &Keypair{Public: priv.Public().(ed25519.PublicKey), Private: priv}, nil
	}
	if b, err := os.ReadFile(path); err == nil {
		raw, err := hex.DecodeString(strings.TrimSpace(string(b)))
		if err != nil {
			return nil, err
		}
		if len(raw) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid Ed25519 private key size")
		}
		priv := ed25519.PrivateKey(raw)
		return &Keypair{Public: priv.Public().(ed25519.PublicKey), Private: priv}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(priv)), 0o600); err != nil {
		return nil, err
	}
	return &Keypair{Public: priv.Public().(ed25519.PublicKey), Private: priv}, nil
}

func SignHex(priv ed25519.PrivateKey, msg []byte) string {
	return hex.EncodeToString(ed25519.Sign(priv, msg))
}

func VerifyHex(pubHex, sigHex string, msg []byte) error {
	pubRaw, err := hex.DecodeString(strings.TrimSpace(pubHex))
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(strings.TrimSpace(sigHex))
	if err != nil {
		return err
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid Ed25519 public key size")
	}
	if !ed25519.Verify(ed25519.PublicKey(pubRaw), msg, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func PublicKeyHex(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pub)
}
