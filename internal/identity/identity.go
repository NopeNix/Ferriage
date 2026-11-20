// Package identity provides the peer's stable node identity.
//
// Each peer generates an ed25519 keypair on first start and persists it
// under "<stateDir>/.ferriage/identity.json" — explicitly outside the
// synced stacks tree, so sync never walks it (ADR 0001). peer_id is the
// hex-encoded public key; it is the LWW tie-breaker and the seed for
// future signing. The private key never leaves the host.
package identity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DirName is the never-synced subdirectory under the state dir that
// holds this peer's own identity and sync ledger.
const DirName = ".ferriage"

// Identity is a peer's keypair plus its stable ID.
type Identity struct {
	Private ed25519.PrivateKey `json:"-"`
	Public  ed25519.PublicKey  `json:"-"`
	id      string
}

// ID returns the peer ID: hex of the public key.
func (i *Identity) ID() string { return i.id }

// file stores the persisted keypair (private key as raw ed25519 seed
// bytes, base64 via json of []byte).
type file struct {
	PublicKey  []byte `json:"public_key"`
	PrivateKey []byte `json:"private_key"`
}

// Dir returns the identity/ledger directory for the given state root.
func Dir(stateRoot string) string {
	return filepath.Join(stateRoot, DirName)
}

// LoadOrCreate returns the identity stored under stateRoot, creating
// and persisting a new keypair on first use.
func LoadOrCreate(stateRoot string) (*Identity, error) {
	dir := Dir(stateRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("identity: create dir: %w", err)
	}
	path := filepath.Join(dir, "identity.json")
	data, err := os.ReadFile(path)
	if err == nil {
		return parse(data)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("identity: read: %w", err)
	}
	idn, err := generate()
	if err != nil {
		return nil, err
	}
	if err := idn.persist(path); err != nil {
		return nil, err
	}
	return idn, nil
}

func parse(data []byte) (*Identity, error) {
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("identity: decode: %w", err)
	}
	if len(f.PrivateKey) != ed25519.PrivateKeySize || len(f.PublicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("identity: invalid keypair sizes")
	}
	priv := ed25519.PrivateKey(f.PrivateKey)
	pub := ed25519.PublicKey(f.PublicKey)
	if !priv.Public().(ed25519.PublicKey).Equal(pub) {
		return nil, fmt.Errorf("identity: public/private key mismatch")
	}
	return &Identity{Private: priv, Public: pub, id: hex.EncodeToString(pub)}, nil
}

func generate() (*Identity, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: generate: %w", err)
	}
	return &Identity{Private: priv, Public: pub, id: hex.EncodeToString(pub)}, nil
}

func (i *Identity) persist(path string) error {
	f := file{PublicKey: []byte(i.Public), PrivateKey: []byte(i.Private)}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("identity: encode: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("identity: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("identity: rename: %w", err)
	}
	return nil
}
