package main

import (
	logiclogging "github.com/jose/matrix-v2/internal/logic/logging"
	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/logic/runtimecheck"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	execprovider "github.com/jose/matrix-v2/internal/providers/exec"
	"github.com/jose/matrix-v2/internal/providers/network"
	"github.com/jose/matrix-v2/internal/providers/osfs"
)

func openLogConfig() (logiclogging.Config, func(), error) {
	mgr, cleanup, err := openReadOnlyConfigManager()
	if err != nil {
		return logiclogging.Config{}, nil, err
	}
	cfg, err := logiclogging.ResolveConfig(mgr)
	if err != nil {
		cleanup()
		return logiclogging.Config{}, nil, err
	}
	return cfg, cleanup, nil
}

func openLogConfigFallback() (logiclogging.Config, func(), []string) {
	cfg, cleanup, err := openLogConfig()
	if err == nil {
		return cfg, cleanup, nil
	}

	fallback := logiclogging.Config{
		Level:      0,
		Format:     "json",
		Sink:       "file",
		FilePath:   "logs/matrix-runtime.jsonl",
		MaxBytes:   10 * 1024 * 1024,
		MaxBackups: 5,
	}
	return fallback, func() {}, []string{"logging config unavailable, using fallback defaults: " + err.Error()}
}

func buildLogsDoctorReport() (map[string]any, error) {
	cfg, cleanup, warnings := openLogConfigFallback()
	defer cleanup()
	return logiclogging.BuildDoctorReport(osfs.NewFSProvider(), cfg, warnings)
}

func buildRuntimeDoctorReport() (map[string]any, error) {
	netProv := network.NewProvider()
	fsProv := osfs.NewFSProvider()
	proc := execprovider.NewProvider()

	provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath)
	if err != nil {
		return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
			VaultPath:   DefaultVaultPath,
			JSONRPCAddr: "127.0.0.1:9090",
			ACPHTTPAddr: "127.0.0.1:9091",
			Net:         netProv,
			FS:          fsProv,
		})
	}

	cfgRdr := osfs.NewConfigProvider()
	registry, err := agentmgr.NewRegistry(cfgRdr, provider)
	if err != nil {
		provider.Close()
		return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
			VaultPath:   DefaultVaultPath,
			JSONRPCAddr: "127.0.0.1:9090",
			ACPHTTPAddr: "127.0.0.1:9091",
			Net:         netProv,
			FS:          fsProv,
		})
	}

	return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
		VaultPath:   DefaultVaultPath,
		JSONRPCAddr: "127.0.0.1:9090",
		ACPHTTPAddr: "127.0.0.1:9091",
		Net:         netProv,
		FS:          fsProv,
		BuildInput: &runtimecheck.BuildInput{
			Store:         provider,
			Registry:      registry,
			Process:       proc,
			ConfigManager: config.NewManager(vault.NewVault(provider)),
			ConfigReader:  cfgRdr,
			Net:           netProv,
			JSONRPCAddr:   "127.0.0.1:9090",
			ACPHTTPAddr:   "127.0.0.1:9091",
		},
	})
}
