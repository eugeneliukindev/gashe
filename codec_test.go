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

func TestRaw(t *testing.T) {
	r := gashe.Raw{}
	for _, in := range []any{[]byte("<p>hi</p>"), "<p>hi</p>"} {
		raw, err := r.Marshal(in)
		if err != nil || string(raw) != "<p>hi</p>" {
			t.Fatalf("Marshal(%T) = %q, %v", in, raw, err)
		}
	}
	var b []byte
	var s string
	if err := r.Unmarshal([]byte("<p>hi</p>"), &b); err != nil || string(b) != "<p>hi</p>" {
		t.Fatalf("Unmarshal []byte = %q, %v", b, err)
	}
	if err := r.Unmarshal([]byte("<p>hi</p>"), &s); err != nil || s != "<p>hi</p>" {
		t.Fatalf("Unmarshal string = %q, %v", s, err)
	}
	in := user{Name: "Ann"}
	raw, _ := r.Marshal(in)
	var out user
	if err := r.Unmarshal(raw, &out); err != nil || out != in || raw[0] != '{' {
		t.Fatalf("other types must go through JSON: %q, %v", raw, err)
	}
}
