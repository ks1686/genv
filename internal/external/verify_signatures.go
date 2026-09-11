package external

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"

	"aead.dev/minisign"
	"github.com/ProtonMail/go-crypto/openpgp"
)

// VerifyMinisign verifies a payload with a pinned minisign public key.
func VerifyMinisign(payload, signature []byte, publicKeyText string) error {
	var publicKey minisign.PublicKey
	if err := publicKey.UnmarshalText([]byte(publicKeyText)); err != nil {
		return fmt.Errorf("parse minisign public key: %w", err)
	}
	if !minisign.Verify(publicKey, payload, signature) {
		return fmt.Errorf("minisign verification failed")
	}
	return nil
}

// VerifyOpenPGP verifies a detached signature and pins the signer's full fingerprint.
func VerifyOpenPGP(payload, signature, publicKey []byte, fingerprint string) error {
	var (
		keyring openpgp.EntityList
		err     error
	)
	if bytes.Contains(publicKey, []byte("BEGIN PGP PUBLIC KEY BLOCK")) {
		keyring, err = openpgp.ReadArmoredKeyRing(bytes.NewReader(publicKey))
	} else {
		keyring, err = openpgp.ReadKeyRing(bytes.NewReader(publicKey))
	}
	if err != nil {
		return fmt.Errorf("parse OpenPGP public key: %w", err)
	}
	var signer *openpgp.Entity
	if bytes.Contains(signature, []byte("BEGIN PGP SIGNATURE")) {
		signer, err = openpgp.CheckArmoredDetachedSignature(keyring, bytes.NewReader(payload), bytes.NewReader(signature), nil)
	} else {
		signer, err = openpgp.CheckDetachedSignature(keyring, bytes.NewReader(payload), bytes.NewReader(signature), nil)
	}
	if err != nil {
		return fmt.Errorf("OpenPGP verification failed: %w", err)
	}
	expected, err := hex.DecodeString(strings.TrimSpace(fingerprint))
	if err != nil || len(expected) != len(signer.PrimaryKey.Fingerprint) {
		return fmt.Errorf("invalid OpenPGP fingerprint")
	}
	if subtle.ConstantTimeCompare(expected, signer.PrimaryKey.Fingerprint) != 1 {
		return fmt.Errorf("OpenPGP signer fingerprint does not match pinned fingerprint")
	}
	return nil
}
