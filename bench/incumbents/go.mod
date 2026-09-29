module pyjsonbench

go 1.27

require (
	github.com/__GH_OWNER__/pyjson v0.0.0
	github.com/docker/go v1.5.1-1
	github.com/goccy/go-json v0.11.1
	github.com/gowebpki/jcs v1.0.2
	github.com/json-iterator/go v1.1.12
	github.com/segmentio/encoding v0.5.4
	github.com/tent/canonical-json-go v0.0.0-20130607151641-96e4ba3a7613
	github.com/ucarion/jcs v0.1.2
	github.com/virtuald/go-pyjson v0.0.0-20210406060839-cfd711b90529
	olympos.io/encoding/cjson v0.0.0-20191103175252-b41f647ea928
)

require (
	github.com/modern-go/concurrent v0.0.0-20180228061459-e0a39a4cb421 // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	golang.org/x/sys v0.0.0-20211110154304-99a53858aa08 // indirect
)

replace github.com/__GH_OWNER__/pyjson => ../..
