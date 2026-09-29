package pyjson_test

import (
	"fmt"

	"github.com/__GH_OWNER__/pyjson"
)

func ExampleDumps() {
	d := pyjson.NewDict()
	d.Set("id", 7)
	d.Set("ratio", 1.0)
	d.Set("big", 1e16)
	out, _ := pyjson.Dumps(d, pyjson.Options{})
	fmt.Println(string(out))
	// Output: {"id": 7, "ratio": 1.0, "big": 1e+16}
}

func ExampleLoads() {
	v, _ := pyjson.Loads([]byte(`{"a": 1, "b": 2, "a": 3}`))
	out, _ := pyjson.Dumps(v, pyjson.Options{Separators: &pyjson.Separators{Item: ",", Key: ":"}})
	fmt.Println(string(out))
	// Output: {"a":3,"b":2}
}
