module github.com/Josepavese/matrix

go 1.26.0

// The toolchain directive is the single source of the Go pin. The CI and release
// workflows read it via actions/setup-go's go-version-file, so the compiler that
// builds published assets cannot drift from the one that qualifies the tree.
// The coverage floors in scripts/quality_gate.sh are calibrated on this toolchain.
// Coverage instrumentation differs between Go releases, so building on a different
// one measures slightly different numbers: with Go 1.25 six packages landed about a
// point below their floors on CI while passing locally on the pinned toolchain.
toolchain go1.27.3

require (
	github.com/a2aproject/a2a-go/v2 v2.7.0
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/rogpeppe/go-internal v1.14.1
	github.com/spf13/cobra v1.10.2
	go.etcd.io/bbolt v1.4.3
	golang.org/x/sys v0.47.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
)
