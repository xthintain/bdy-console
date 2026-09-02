package timeline

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompressRoundTrip(t *testing.T) {
	data := []byte(strings.Repeat("hello world ", 1000))
	compressed, err := CompressPayload(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(compressed) >= len(data) {
		t.Fatalf("compressed not smaller: %d >= %d", len(compressed), len(data))
	}
	back, err := DecompressPayload(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, data) {
		t.Fatal("round trip mismatch")
	}
}

func TestCompressIfNeededAlwaysRoundTrips(t *testing.T) {
	// Even incompressible-looking data must round trip; compressIfNeeded picks
	// the smaller representation but both must decompress correctly.
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i % 251)
	}
	out, comp, err := compressIfNeeded(data, 1)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decompressIfNeeded(out, comp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, data) {
		t.Fatal("round trip mismatch for incompressible data")
	}
}

func TestCompressIfNeededCompressesText(t *testing.T) {
	data := []byte(strings.Repeat("aaaa", 4096))
	out, comp, err := compressIfNeeded(data, 1)
	if err != nil {
		t.Fatal(err)
	}
	if comp == 0 {
		t.Fatal("expected compression for repeated text")
	}
	back, err := decompressIfNeeded(out, comp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(back, data) {
		t.Fatal("decompress mismatch")
	}
}
