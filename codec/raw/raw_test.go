package raw_test

import (
	"testing"

	"github.com/eugeneliukindev/gashe/codec/raw"
)

type user struct {
	Name string
}

func TestRaw(t *testing.T) {
	r := raw.Codec{}
	for _, in := range []any{[]byte("<p>hi</p>"), "<p>hi</p>"} {
		data, err := r.Marshal(in)
		if err != nil || string(data) != "<p>hi</p>" {
			t.Fatalf("Marshal(%T) = %q, %v", in, data, err)
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
	data, err := r.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out user
	if err := r.Unmarshal(data, &out); err != nil || out != in || data[0] != '{' {
		t.Fatalf("other types must go through JSON: %q, %v", data, err)
	}
}
