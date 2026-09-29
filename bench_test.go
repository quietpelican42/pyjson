package pyjson

import (
	"encoding/json"
	"testing"
)

func benchDoc() (goVal map[string]any, pyVal *Dict) {
	items := make([]any, 50)
	pyItems := make([]any, 50)
	for i := range items {
		items[i] = map[string]any{"id": i, "name": "item é", "price": float64(i) * 1.25, "tags": []any{"a", "b", "c"}, "ok": true}
		pyItems[i] = mustDict("id", i, "name", "item é", "price", float64(i)*1.25, "tags", []any{"a", "b", "c"}, "ok", true)
	}
	return map[string]any{"items": items, "count": 50}, mustDict("items", pyItems, "count", 50)
}

func BenchmarkDumps(b *testing.B) {
	_, doc := benchDoc()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Dumps(doc, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDumpsSortKeys(b *testing.B) {
	_, doc := benchDoc()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Dumps(doc, Options{SortKeys: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodingJSONMarshal(b *testing.B) {
	doc, _ := benchDoc()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(doc); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoads(b *testing.B) {
	_, doc := benchDoc()
	data, _ := Dumps(doc, Options{})
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Loads(data); err != nil {
			b.Fatal(err)
		}
	}
}
