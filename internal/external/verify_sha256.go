package external

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// VerifySHA256 compares an observed digest with an expected SHA-256 value.
func VerifySHA256(observed, expected string) error {
	expected = strings.TrimSpace(expected)
	if prefix, value, ok := strings.Cut(expected, ":"); ok {
		if !strings.EqualFold(prefix, "sha256") {
			return fmt.Errorf("unsupported digest algorithm %q", prefix)
		}
		expected = value
	}
	observedBytes, err := hex.DecodeString(strings.ToLower(observed))
	if err != nil || len(observedBytes) != sha256Bytes {
		return fmt.Errorf("invalid observed SHA-256 digest")
	}
	expectedBytes, err := hex.DecodeString(strings.ToLower(expected))
	if err != nil || len(expectedBytes) != sha256Bytes {
		return fmt.Errorf("invalid expected SHA-256 digest")
	}
	if subtle.ConstantTimeCompare(observedBytes, expectedBytes) != 1 {
		return fmt.Errorf("SHA-256 verification failed")
	}
	return nil
}

const sha256Bytes = 32

// ChecksumForAsset returns the unique SHA-256 associated with assetName.
func ChecksumForAsset(body []byte, assetName string) (string, error) {
	var match string
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		if match != "" {
			return "", fmt.Errorf("checksum file contains duplicate entries for %q", assetName)
		}
		match = fields[0]
	}
	if match == "" {
		return "", fmt.Errorf("checksum file has no entry for %q", assetName)
	}
	if _, err := hex.DecodeString(match); err != nil || len(match) != sha256Bytes*2 {
		return "", fmt.Errorf("checksum for %q is not SHA-256", assetName)
	}
	return strings.ToLower(match), nil
}
