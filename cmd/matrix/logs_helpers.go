package main

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/logic/config"
	logiclogging "github.com/jose/matrix-v2/internal/logic/logging"
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
	jsonrpcAddr, acpHTTPAddr, a2aHTTPAddr := discoverRuntimeAddrs()

	provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath)
	if err != nil {
		return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
			VaultPath:   DefaultVaultPath,
			JSONRPCAddr: jsonrpcAddr,
			ACPHTTPAddr: acpHTTPAddr,
			A2AHTTPAddr: a2aHTTPAddr,
			Net:         netProv,
			FS:          fsProv,
		})
	}
	defer func() { _ = provider.Close() }()

	cfgMgr := config.NewManager(vault.NewVault(provider))
	jsonrpcAddr = cfgMgr.GetWithDefault("jsonrpc_addr", jsonrpcAddr)
	acpHTTPAddr = cfgMgr.GetWithDefault("acp_http_addr", acpHTTPAddr)
	a2aHTTPAddr = acpHTTPAddr

	cfgRdr := osfs.NewConfigProvider()
	registry, err := agentmgr.NewRegistry(cfgRdr, provider)
	if err != nil {
		return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
			VaultPath:   DefaultVaultPath,
			JSONRPCAddr: jsonrpcAddr,
			ACPHTTPAddr: acpHTTPAddr,
			A2AHTTPAddr: a2aHTTPAddr,
			Net:         netProv,
			FS:          fsProv,
		})
	}

	return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
		VaultPath:   DefaultVaultPath,
		JSONRPCAddr: jsonrpcAddr,
		ACPHTTPAddr: acpHTTPAddr,
		A2AHTTPAddr: a2aHTTPAddr,
		Net:         netProv,
		FS:          fsProv,
		BuildInput: &runtimecheck.BuildInput{
			Store:         provider,
			Registry:      registry,
			Process:       proc,
			ConfigManager: cfgMgr,
			ConfigReader:  cfgRdr,
			Net:           netProv,
			JSONRPCAddr:   jsonrpcAddr,
			ACPHTTPAddr:   acpHTTPAddr,
			A2AHTTPAddr:   a2aHTTPAddr,
		},
	})
}

func discoverRuntimeAddrs() (string, string, string) {
	jsonrpcAddr := DefaultJSONRPCAddr
	acpHTTPAddr := DefaultACPHTTPAddr
	a2aHTTPAddr := DefaultACPHTTPAddr

	cfg, cleanup, _ := openLogConfigFallback()
	defer cleanup()
	if cfg.FilePath == "" {
		return jsonrpcAddr, acpHTTPAddr, a2aHTTPAddr
	}

	file, err := os.Open(cfg.FilePath)
	if err != nil {
		return jsonrpcAddr, acpHTTPAddr, a2aHTTPAddr
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry struct {
			Event string `json:"event"`
			Addr  string `json:"addr"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		switch entry.Event {
		case "jsonrpc_daemon_starting":
			if entry.Addr != "" {
				jsonrpcAddr = entry.Addr
			}
		case "acp_http_starting":
			if entry.Addr != "" {
				acpHTTPAddr = entry.Addr
				a2aHTTPAddr = entry.Addr
			}
		}
	}

	return jsonrpcAddr, acpHTTPAddr, a2aHTTPAddr
}
