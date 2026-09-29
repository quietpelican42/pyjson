// Command compare prints encoding/json and pyjson output side by side.
package main

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/__GH_OWNER__/pyjson"
)

func main() {
	d := pyjson.NewDict()
	d.Set("b", 1.0)
	d.Set("a", []any{1e16, 1e-5, math.Copysign(0, -1)})
	d.Set("s", "café <b>")

	std, err := json.Marshal(map[string]any{"b": 1.0, "a": []any{1e16, 1e-5, math.Copysign(0, -1)}, "s": "café <b>"})
	if err != nil {
		panic(err)
	}
	py, err := pyjson.Dumps(d, pyjson.Options{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("encoding/json: %s\n", std)
	fmt.Printf("pyjson:        %s\n", py)
}
