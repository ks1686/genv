package external

import (
	"fmt"
	"os"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// VerifySigstore verifies an artifact bundle against a pinned Fulcio identity.
func VerifySigstore(payloadPath, bundlePath, identity, issuer string) error {
	b, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return fmt.Errorf("load Sigstore bundle: %w", err)
	}
	trustedRoot, err := root.FetchTrustedRoot()
	if err != nil {
		return fmt.Errorf("load Sigstore trusted root: %w", err)
	}
	verifier, err := verify.NewVerifier(trustedRoot,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	)
	if err != nil {
		return fmt.Errorf("create Sigstore verifier: %w", err)
	}
	certificateIdentity, err := verify.NewShortCertificateIdentity(issuer, "", identity, "")
	if err != nil {
		return fmt.Errorf("create Sigstore identity policy: %w", err)
	}
	payload, err := os.Open(payloadPath)
	if err != nil {
		return fmt.Errorf("open Sigstore artifact: %w", err)
	}
	defer payload.Close()
	_, err = verifier.Verify(b, verify.NewPolicy(verify.WithArtifact(payload), verify.WithCertificateIdentity(certificateIdentity)))
	if err != nil {
		return fmt.Errorf("Sigstore verification failed: %w", err)
	}
	return nil
}
