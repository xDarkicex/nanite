module github.com/xDarkicex/nanite/sse

go 1.25.7

require (
	github.com/quic-go/quic-go v0.60.0
	github.com/xDarkicex/memory v1.0.37
	github.com/xDarkicex/nanite v0.0.0
	github.com/xDarkicex/nanite/quic v0.0.0-00010101000000-000000000000
)

replace (
	github.com/xDarkicex/nanite => ../
	github.com/xDarkicex/nanite/quic => ../quic
)

require (
	github.com/quic-go/qpack v0.6.0 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/net v0.55.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
)
