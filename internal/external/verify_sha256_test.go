package external

import "testing"

func TestVerifySHA256(t *testing.T) {
	const digest = "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5"
	if err := VerifySHA256(digest, "SHA256:"+digest); err != nil {
		t.Fatalf("VerifySHA256() error: %v", err)
	}
	if err := VerifySHA256(digest, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("VerifySHA256() accepted wrong digest")
	}
}

func TestChecksumForAssetRequiresExactUniqueName(t *testing.T) {
	body := []byte("239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5  tool.tar.gz\n")
	got, err := ChecksumForAsset(body, "tool.tar.gz")
	if err != nil {
		t.Fatalf("ChecksumForAsset() error: %v", err)
	}
	if got != "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5" {
		t.Fatalf("checksum = %q", got)
	}
	if _, err := ChecksumForAsset(append(body, body...), "tool.tar.gz"); err == nil {
		t.Fatal("duplicate checksum accepted")
	}
}
