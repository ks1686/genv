package external

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"
	"github.com/ProtonMail/go-crypto/openpgp"
)

func TestVerifySigstoreRejectsMalformedBundle(t *testing.T) {
	payload := filepath.Join(t.TempDir(), "payload")
	bundle := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(payload, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundle, []byte(`{"invalid":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifySigstore(payload, bundle, "https://github.com/owner/repo/.github/workflows/release.yml@refs/heads/main", "https://token.actions.githubusercontent.com"); err == nil {
		t.Fatal("malformed bundle accepted")
	}
}

func TestVerifyMinisign(t *testing.T) {
	publicKey, privateKey, err := minisign.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicText, err := publicKey.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("signed payload")
	signature := minisign.Sign(privateKey, payload)
	if err := VerifyMinisign(payload, signature, string(publicText)); err != nil {
		t.Fatalf("VerifyMinisign() error: %v", err)
	}
	if err := VerifyMinisign([]byte("modified"), signature, string(publicText)); err == nil {
		t.Fatal("modified payload accepted")
	}
}

func TestVerifyOpenPGPPinsFingerprint(t *testing.T) {
	entity, err := openpgp.NewEntity("test", "", "test@example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	var publicKey bytes.Buffer
	if err := entity.Serialize(&publicKey); err != nil {
		t.Fatal(err)
	}
	payload := []byte("signed payload")
	var signature bytes.Buffer
	if err := openpgp.DetachSign(&signature, entity, bytes.NewReader(payload), nil); err != nil {
		t.Fatal(err)
	}
	fingerprint := strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint))
	if err := VerifyOpenPGP(payload, signature.Bytes(), publicKey.Bytes(), fingerprint); err != nil {
		t.Fatalf("VerifyOpenPGP() error: %v", err)
	}
	if err := VerifyOpenPGP(payload, signature.Bytes(), publicKey.Bytes(), strings.Repeat("0", len(fingerprint))); err == nil {
		t.Fatal("wrong fingerprint accepted")
	}
}
