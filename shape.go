package gashe

import (
	"hash/fnv"
	"reflect"
	"strings"
)

// shapeOf returns a hash of the form of t: the exported field names, json and msgpack tags and
// types, nested types included. A stored result keeps the hash of its type, so after a deploy that changes the
// type, old entries are misses instead of decoding into empty fields.
func (c *Cache) shapeOf(t reflect.Type) uint64 {
	if h, ok := c.shapes.Load(t); ok {
		return h.(uint64) //nolint:forcetypeassert // only shapeOf stores, and it stores uint64
	}
	var b strings.Builder
	writeShape(&b, t, map[reflect.Type]bool{})
	h := fnv.New64a()
	_, _ = h.Write([]byte(b.String()))
	sum := h.Sum64()
	c.shapes.Store(t, sum)
	return sum
}

// writeShape writes the form of t; seen stops recursive types.
func writeShape(b *strings.Builder, t reflect.Type, seen map[reflect.Type]bool) {
	switch t.Kind() { //nolint:exhaustive // every other kind is described by its name
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Chan:
		b.WriteString(t.Kind().String() + "(")
		writeShape(b, t.Elem(), seen)
		b.WriteString(")")
	case reflect.Map:
		b.WriteString("map(")
		writeShape(b, t.Key(), seen)
		b.WriteString(",")
		writeShape(b, t.Elem(), seen)
		b.WriteString(")")
	case reflect.Struct:
		if seen[t] {
			b.WriteString(t.String())
			return
		}
		seen[t] = true
		b.WriteString("{")
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue // encoding/json does not store it
			}
			b.WriteString(f.Name + " " + f.Tag.Get("json") + " ")
			if packed := f.Tag.Get("msgpack"); packed != "" {
				// The msgpack codec names fields by this tag. Only a set tag is written, so the
				// shapes of types without one stay as they were before msgpack tags counted.
				b.WriteString("msgpack:" + packed + " ")
			}
			writeShape(b, f.Type, seen)
			b.WriteString(";")
		}
		b.WriteString("}")
	default:
		b.WriteString(t.String())
	}
}
