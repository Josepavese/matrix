package main

import (
	"fmt"
	"os"

	"github.com/jose/matrix-v2/internal/logic/agentmgr"
	"github.com/jose/matrix-v2/internal/logic/config"
	"github.com/jose/matrix-v2/internal/logic/vault"
	"github.com/jose/matrix-v2/internal/middleware"
	"github.com/jose/matrix-v2/internal/providers/bolt"
	execprovider "github.com/jose/matrix-v2/internal/providers/exec"
	networkprovider "github.com/jose/matrix-v2/internal/providers/network"
	"github.com/jose/matrix-v2/internal/providers/osfs"
)

// AppContext holds shared dependencies for CLI commands that need vault access.
// It eliminates duplicated provider wiring across cmd files.
type AppContext struct {
	Store     middleware.Storage
	Vault     *vault.Vault
	Config    *config.Manager
	ConfigRdr middleware.ConfigReader
	FS        middleware.FS
	closeFn   func()
}

// AgentContext holds dependencies for agent-related commands.
type AgentContext struct {
	Store    middleware.Storage
	Registry *agentmgr.Registry
	ConfigRdr middleware.ConfigReader
	closeFn  func()
}

// InstallerContext holds dependencies for install/uninstall commands.
type InstallerContext struct {
	Store     middleware.Storage
	Installer *agentmgr.Installer
	closeFn   func()
}

// NewAppContext opens the vault in read-write mode and builds core dependencies.
func NewAppContext(vaultPath string) (*AppContext, func(), error) {
	provider, err := bolt.NewProvider(vaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	closeFn := func() { _ = provider.Close() }
	v := vault.NewVault(provider)
	cfgMgr := config.NewManager(v)
	return &AppContext{
		Store:     provider,
		Vault:     v,
		Config:    cfgMgr,
		ConfigRdr: osfs.NewConfigProvider(),
		FS:        osfs.NewFSProvider(),
		closeFn:   closeFn,
	}, closeFn, nil
}

// NewReadOnlyAppContext opens the vault in read-only mode.
func NewReadOnlyAppContext(vaultPath string) (*AppContext, func(), error) {
	provider, err := bolt.NewReadOnlyProvider(vaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	closeFn := func() { _ = provider.Close() }
	v := vault.NewVault(provider)
	cfgMgr := config.NewManager(v)
	return &AppContext{
		Store:     provider,
		Vault:     v,
		Config:    cfgMgr,
		ConfigRdr: osfs.NewConfigProvider(),
		FS:        osfs.NewFSProvider(),
		closeFn:   closeFn,
	}, closeFn, nil
}

// NewAgentContext opens read-only storage with agent registry.
func NewAgentContext(vaultPath string) (*AgentContext, func(), error) {
	provider, err := bolt.NewReadOnlyProvider(vaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	closeFn := func() { _ = provider.Close() }
	configRdr := osfs.NewConfigProvider()
	registry, err := agentmgr.NewRegistry(configRdr, provider)
	if err != nil {
		provider.Close()
		return nil, nil, fmt.Errorf("registry error: %w", err)
	}
	return &AgentContext{
		Store:     provider,
		Registry:  registry,
		ConfigRdr: configRdr,
		closeFn:   closeFn,
	}, closeFn, nil
}

// NewAgentStoreContext opens read-write storage for agent mutations.
func NewAgentStoreContext(vaultPath string) (*AgentContext, func(), error) {
	provider, err := bolt.NewProvider(vaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	closeFn := func() { _ = provider.Close() }
	configRdr := osfs.NewConfigProvider()
	registry, err := agentmgr.NewRegistry(configRdr, provider)
	if err != nil {
		provider.Close()
		return nil, nil, fmt.Errorf("registry error: %w", err)
	}
	return &AgentContext{
		Store:     provider,
		Registry:  registry,
		ConfigRdr: configRdr,
		closeFn:   closeFn,
	}, closeFn, nil
}

// NewInstallerContext builds installer dependencies.
func NewInstallerContext(vaultPath string) (*InstallerContext, func(), error) {
	provider, err := bolt.NewProvider(vaultPath)
	if err != nil {
		return nil, nil, fmt.Errorf("vault error: %w", err)
	}
	closeFn := func() { _ = provider.Close() }
	netProv := networkprovider.NewProvider()
	installer, err := agentmgr.NewInstaller(agentmgr.InstallerConfig{
		Net:      netProv,
		Archive:  osfs.NewArchiveProvider(),
		Storage:  provider,
		FS:       osfs.NewFSProvider(),
		Registry: agentmgr.NewCachingRegistryClient(netProv, "", provider),
		BaseDir:  "",
	})
	if err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("installer init error: %w", err)
	}
	return &InstallerContext{
		Store:     provider,
		Installer: installer,
		closeFn:   closeFn,
	}, closeFn, nil
}

// NewDaemonContext builds all dependencies needed for `matrix run`.
func NewDaemonContext(vaultPath string) (*DaemonContext, func(), error) {
	app, closeApp, err := NewAppContext(vaultPath)
	if err != nil {
		return nil, nil, err
	}

	configMgr := osfs.NewConfigProvider()
	execProv := execprovider.NewProvider()
	netProv := networkprovider.NewProvider()

	// Seed pre-installed agents from configs/agents.local.json into vault
	if err := agentmgr.SeedFromConfigFile(app.Store, configMgr, "configs/agents.local.json"); err != nil {
		closeApp()
		return nil, nil, fmt.Errorf("agent seed error: %w", err)
	}

	registry, err := agentmgr.NewRegistry(configMgr, app.Store)
	if err != nil {
		closeApp()
		return nil, nil, fmt.Errorf("registry error: %w", err)
	}

	supervisor := agentmgr.NewSupervisor(execProv, netProv, app.Store, registry)

	archiveProv := osfs.NewArchiveProvider()
	regClient := agentmgr.NewCachingRegistryClient(netProv, "", app.Store)
	installer, err := agentmgr.NewInstaller(agentmgr.InstallerConfig{
		Net:      netProv,
		Archive:  archiveProv,
		Storage:  app.Store,
		FS:       osfs.NewFSProvider(),
		Registry: regClient,
		BaseDir:  "",
	})
	if err != nil {
		closeApp()
		return nil, nil, fmt.Errorf("installer init error: %w", err)
	}

	closeAll := func() {
		closeApp()
	}

	return &DaemonContext{
		App:        app,
		ConfigMgr:  configMgr,
		ExecProv:   execProv,
		NetProv:    netProv,
		Registry:   registry,
		Supervisor: supervisor,
		Installer:  installer,
		closeFn:    closeAll,
	}, closeAll, nil
}

// DaemonContext holds all dependencies for the daemon command.
type DaemonContext struct {
	App        *AppContext
	ConfigMgr  middleware.ConfigManager
	ExecProv   middleware.Process
	NetProv    *networkprovider.Provider
	Registry   *agentmgr.Registry
	Supervisor *agentmgr.Supervisor
	Installer  *agentmgr.Installer
	closeFn    func()
}

// exitf prints an error message to stderr and exits.
func exitf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
