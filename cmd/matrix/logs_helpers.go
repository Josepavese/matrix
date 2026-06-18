package main

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/Josepavese/matrix/internal/logic/agentmgr"
	"github.com/Josepavese/matrix/internal/logic/config"
	logiclogging "github.com/Josepavese/matrix/internal/logic/logging"
	"github.com/Josepavese/matrix/internal/logic/runtimecheck"
	"github.com/Josepavese/matrix/internal/logic/vault"
	"github.com/Josepavese/matrix/internal/providers/bolt"
	execprovider "github.com/Josepavese/matrix/internal/providers/exec"
	"github.com/Josepavese/matrix/internal/providers/network"
	"github.com/Josepavese/matrix/internal/providers/osfs"
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
	jsonrpcAddr, matrixHTTPAddr, a2aHTTPAddr := discoverRuntimeAddrs()

	provider, err := bolt.NewReadOnlyProvider(DefaultVaultPath)
	if err != nil {
		return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
			VaultPath:      DefaultVaultPath,
			JSONRPCAddr:    jsonrpcAddr,
			MatrixHTTPAddr: matrixHTTPAddr,
			A2AHTTPAddr:    a2aHTTPAddr,
			Net:            netProv,
			FS:             fsProv,
		})
	}
	defer func() { _ = provider.Close() }()

	cfgMgr := config.NewManager(vault.NewVault(provider))
	jsonrpcAddr = cfgMgr.GetWithDefault("jsonrpc_addr", jsonrpcAddr)
	matrixHTTPAddr = cfgMgr.GetWithDefault("matrix_http_addr", matrixHTTPAddr)
	a2aHTTPAddr = matrixHTTPAddr

	cfgRdr := config.NewFirstRunConfigReader(osfs.NewConfigProvider())
	registry, err := agentmgr.NewRegistry(cfgRdr, provider)
	if err != nil {
		return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
			VaultPath:      DefaultVaultPath,
			JSONRPCAddr:    jsonrpcAddr,
			MatrixHTTPAddr: matrixHTTPAddr,
			A2AHTTPAddr:    a2aHTTPAddr,
			Net:            netProv,
			FS:             fsProv,
		})
	}

	return runtimecheck.BuildLocalReport(runtimecheck.LocalInput{
		VaultPath:      DefaultVaultPath,
		JSONRPCAddr:    jsonrpcAddr,
		MatrixHTTPAddr: matrixHTTPAddr,
		A2AHTTPAddr:    a2aHTTPAddr,
		Net:            netProv,
		FS:             fsProv,
		BuildInput: &runtimecheck.BuildInput{
			Store:          provider,
			Registry:       registry,
			Process:        proc,
			ConfigManager:  cfgMgr,
			ConfigReader:   cfgRdr,
			Net:            netProv,
			JSONRPCAddr:    jsonrpcAddr,
			MatrixHTTPAddr: matrixHTTPAddr,
			A2AHTTPAddr:    a2aHTTPAddr,
		},
	})
}

func discoverRuntimeAddrs() (string, string, string) {
	jsonrpcAddr := DefaultJSONRPCAddr
	matrixHTTPAddr := DefaultMatrixHTTPAddr
	a2aHTTPAddr := DefaultMatrixHTTPAddr

	cfg, cleanup, _ := openLogConfigFallback()
	defer cleanup()
	if cfg.FilePath == "" {
		return jsonrpcAddr, matrixHTTPAddr, a2aHTTPAddr
	}

	file, err := os.Open(cfg.FilePath)
	if err != nil {
		return jsonrpcAddr, matrixHTTPAddr, a2aHTTPAddr
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
		case "matrix_http_starting":
			if entry.Addr != "" {
				matrixHTTPAddr = entry.Addr
				a2aHTTPAddr = entry.Addr
			}
		}
	}

	return jsonrpcAddr, matrixHTTPAddr, a2aHTTPAddr
}
