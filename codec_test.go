package gashe_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eugeneliukindev/gashe"
)

func TestGzip(t *testing.T) {
	g := gashe.Gzip{}
	in := user{Name: strings.Repeat("Ann", 1000)}
	raw, err := g.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := gashe.JSON{}.Marshal(in)
	if len(raw) >= len(plain) || bytes.Contains(raw, []byte("AnnAnn")) {
		t.Fatalf("not compressed: %d bytes vs %d", len(raw), len(plain))
	}
	var out user
	if err := g.Unmarshal(raw, &out); err != nil || out != in {
		t.Fatalf("round trip: %v", err)
	}
	if err := g.Unmarshal([]byte("not gzip"), &out); err == nil {
		t.Fatal("garbage decoded")
	}
}
