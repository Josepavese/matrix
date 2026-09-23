module github.com/Josepavese/matrix

go 1.25.0

// The coverage floors in scripts/quality_gate.sh are calibrated on this toolchain.
// Coverage instrumentation differs between Go releases, so building on a different
// one measures slightly different numbers: with Go 1.25 six packages landed about a
// point below their floors on CI while passing locally on 1.27.1.
toolchain go1.27.1

require (
	github.com/a2aproject/a2a-go/v2 v2.5.0
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/hanwen/go-fuse/v2 v2.9.0
	github.com/rogpeppe/go-internal v1.14.1
	github.com/spf13/cobra v1.10.2
	go.etcd.io/bbolt v1.4.3
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/mod v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/tools v0.43.0 // indirect
)
