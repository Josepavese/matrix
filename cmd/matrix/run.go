package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Josepavese/matrix/internal/logic/agentcatalog"
	"github.com/Josepavese/matrix/internal/logic/agentdiscovery"
	"github.com/Josepavese/matrix/internal/logic/channelcfg"
	"github.com/Josepavese/matrix/internal/logic/channelruntime"
	"github.com/Josepavese/matrix/internal/logic/config"
	"github.com/Josepavese/matrix/internal/logic/daemon"
	"github.com/Josepavese/matrix/internal/logic/logging"
	"github.com/Josepavese/matrix/internal/logic/onboarding"
	"github.com/Josepavese/matrix/internal/logic/runtimecheck"
	"github.com/Josepavese/matrix/internal/logic/session"
	"github.com/Josepavese/matrix/internal/logic/system_tools"
	matrixa2a "github.com/Josepavese/matrix/internal/providers/a2a"
	"github.com/Josepavese/matrix/internal/providers/agents"
	"github.com/Josepavese/matrix/internal/providers/matrixapi"
	"github.com/Josepavese/matrix/internal/providers/osfs"
	"github.com/Josepavese/matrix/internal/providers/oslog"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the Matrix background daemon",
	RunE: func(_ *cobra.Command, _ []string) error {
		ctx, cancel := signalContext()
		defer cancel()

		d, closeDaemon, err := NewDaemonContext(DefaultVaultPath)
		if err != nil {
			exitf("Daemon init error: %v", err)
		}
		defer closeDaemon()

		// Logging bootstrap
		logRuntime, err := logging.BootstrapWithFactory(d.App.Config, oslog.NewFactory())
		if err != nil {
			exitf("Logging init error: %v", err)
		}
		defer func() {
			if err := logRuntime.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "Logging shutdown error: %v\n", err)
			}
		}()
		log := slog.Default().With("component", "runtime")
		log.Info("logging initialized", "event", "logging_initialized", "sink", logRuntime.Config.Sink, "format", logRuntime.Config.Format, "file", logRuntime.Config.FilePath, "level", logRuntime.Config.Level.String(), "max_bytes", logRuntime.Config.MaxBytes, "max_backups", logRuntime.Config.MaxBackups, "acp_wire", logRuntime.Config.ACPWire)

		// Start agents
		if err := d.Supervisor.StartAll(ctx); err != nil {
			log.Warn("apm failed to start agents", "event", "apm_start_failed", "error", err)
		}

		// Session management
		sysTools := system_tools.NewHandler(d.ConfigMgr, d.App.Store, d.Installer)
		localizer := osfs.NewLocalizer(DefaultLocalesPath, DefaultLocale)
		discoverySources := parseDiscoverySources(d.App.Config.GetWithDefault("onboarding.discovery.sources", "local,acp_registry"))
		a2aCatalogURLs := splitCommaList(d.App.Config.GetWithDefault("onboarding.discovery.a2a_catalog_urls", ""))
		catalog := agentcatalog.NewService(agentcatalog.Config{
			Storage:        d.App.Store,
			Net:            d.NetProv,
			Installer:      d.Installer,
			Sources:        discoverySources,
			A2ACatalogURLs: a2aCatalogURLs,
		})
		wizard := onboarding.NewWizard(onboarding.WizardDependencies{
			Storage:   d.App.Store,
			Config:    d.ConfigMgr,
			Localizer: localizer,
			Proc:      d.ExecProv,
			Installer: d.Installer,
			Discovery: catalog,
			Activator: catalog,
			FS:        d.App.FS,
			Net:       d.NetProv,
		})
		agentRouter := agents.NewRouter(d.Supervisor)
		agentRouter.SetTrustMode(func() bool {
			return d.App.Config.GetWithDefault("agent.trust_mode", "false") == "true"
		})
		homeDir, err := d.App.FS.UserHomeDir()
		if err != nil {
			log.Error("failed to determine user home directory", "error", err)
			homeDir = "."
		}
		agentRouter.SetFS(d.App.FS, homeDir)
		agentRouter.SetProcess(d.ExecProv)
		agentRouter.StartKeepalive(ctx)
		sessionMgr := session.NewManager(d.App.Store, agentRouter, wizard, sysTools)
		sessionMgr.SetEndpointResolver(d.Supervisor)
		if agent := d.App.Config.GetWithDefault("default_agent", ""); agent != "" {
			sessionMgr.SetDefaultAgent(agent)
		}
		if agent := d.App.Config.GetWithDefault("action_agent", ""); agent != "" {
			sessionMgr.SetActionAgent(agent)
		}

		// Configurable addresses with defaults
		jsonrpcAddr := d.App.Config.GetWithDefault("jsonrpc_addr", DefaultJSONRPCAddr)
		matrixHTTPAddr := d.App.Config.GetWithDefault("matrix_http_addr", DefaultMatrixHTTPAddr)
		matrixAPIKey := d.App.Config.GetWithDefault("matrix_api_key", "")
		daemonAPIKey := d.App.Config.GetWithDefault("daemon_api_key", "")
		if err := runtimecheck.RequireAPIKeyForExternalBind(matrixHTTPAddr, matrixAPIKey, "matrix_http_addr", "matrix_api_key"); err != nil {
			log.Error("refusing unsafe matrix http bind", "event", "matrix_http_unsafe_bind", "error", err)
			return err
		}
		if err := runtimecheck.RequireAPIKeyForExternalBind(jsonrpcAddr, daemonAPIKey, "jsonrpc_addr", "daemon_api_key"); err != nil {
			log.Error("refusing unsafe json-rpc bind", "event", "jsonrpc_unsafe_bind", "error", err)
			return err
		}

		// Matrix HTTP API
		matrixAPIServer := matrixapi.NewServer(sessionMgr)
		matrixAPIServer.WithTraceStorage(d.App.Store)
		matrixAPIServer.WithEndpointResolver(d.Supervisor)
		matrixAPIServer.WithDefaultAgent(d.App.Config.GetWithDefault("default_agent", DefaultAgent))
		if matrixAPIKey != "" {
			matrixAPIServer.WithAPIKey(matrixAPIKey)
			log.Info("matrix api key configured", "event", "matrix_apikey_set")
		}
		mux := http.NewServeMux()
		matrixAPIServer.RegisterRoutes(mux)
		go matrixAPIServer.StartRunSinkDeliveryWorker(ctx)
		a2aServer := matrixa2a.NewServer(sessionMgr, "http://"+matrixHTTPAddr, d.App.Config.GetWithDefault("default_agent", DefaultAgent))
		if matrixAPIKey != "" {
			a2aServer.WithAPIKey(matrixAPIKey)
		}
		a2aServer.RegisterRoutes(mux)
		mux.HandleFunc("/_matrix/runtime", func(w http.ResponseWriter, r *http.Request) {
			if !authorizeMatrixRuntimeRequest(w, r, matrixAPIKey) {
				return
			}
			report, err := runtimecheck.BuildReport(runtimecheck.BuildInput{
				Store:          d.App.Store,
				Registry:       d.Registry,
				Process:        d.ExecProv,
				ConfigManager:  d.App.Config,
				ConfigReader:   config.NewFirstRunConfigReader(d.ConfigMgr),
				Net:            d.NetProv,
				JSONRPCAddr:    jsonrpcAddr,
				MatrixHTTPAddr: matrixHTTPAddr,
				A2AHTTPAddr:    matrixHTTPAddr,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(report); err != nil {
				log.Warn("failed to encode runtime report", "event", "runtime_report_encode_failed", "error", err)
			}
		})

		// JSON-RPC Daemon
		srv := daemon.NewServer(d.App.Vault, d.NetProv)
		if daemonAPIKey != "" {
			srv.WithAPIKey(daemonAPIKey)
			log.Info("daemon API key configured", "event", "daemon_apikey_set")
		}

		// Messaging gateways (telegram and future providers)
		if tgCfg, tgSource, err := channelcfg.LoadTelegramConfig(d.ConfigMgr, d.App.Config); err == nil {
			log.Info("telegram configuration resolved", "event", "telegram_config_resolved", "enabled", tgCfg.Enabled, "configured", tgCfg.Token != "", "source", tgSource)
		}
		gateways, err := channelruntime.StartAll(ctx, d.ConfigMgr, d.App.Config, sessionMgr, channelruntime.DefaultFactories()...)
		if err != nil {
			log.Error("channel runtime failed to start", "event", "channel_runtime_failed", "error", err)
		}

		// Start Matrix HTTP API server
		httpServer := &http.Server{Addr: matrixHTTPAddr, Handler: matrixapi.LocalCORSHandler(mux)}
		go func() {
			log.Info("starting matrix http server", "event", "matrix_http_starting", "addr", matrixHTTPAddr)
			if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
				log.Error("matrix http server stopped with error", "event", "matrix_http_stopped", "error", err, "addr", matrixHTTPAddr)
			}
		}()

		// Graceful shutdown: wait for signal then shut down HTTP server + agent router
		go func() {
			<-ctx.Done()
			log.Info("shutting down...", "event", "shutdown_started")
			if err := channelruntime.StopAll(gateways); err != nil {
				log.Warn("channel runtime shutdown error", "event", "channel_runtime_shutdown_error", "error", err)
			}
			agentRouter.Close()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := httpServer.Shutdown(shutdownCtx); err != nil {
				log.Warn("http server shutdown error", "event", "http_shutdown_error", "error", err)
			}
		}()

		// Start JSON-RPC Server (blocks until context cancelled)
		log.Info("starting matrix jsonrpc daemon", "event", "jsonrpc_daemon_starting", "addr", jsonrpcAddr)
		if err := srv.Start(ctx, jsonrpcAddr); err != nil {
			log.Error("jsonrpc daemon stopped with error", "event", "jsonrpc_daemon_stopped", "error", err, "addr", jsonrpcAddr)
			cancel()
			os.Exit(1)
		}
		log.Info("matrix daemon exited cleanly", "event", "daemon_exited")
		cancel()
		return nil
	},
}

func parseDiscoverySources(raw string) []agentdiscovery.Source {
	values := splitCommaList(raw)
	if len(values) == 0 {
		return agentcatalog.DefaultSources()
	}
	sources := make([]agentdiscovery.Source, 0, len(values))
	for _, value := range values {
		switch agentdiscovery.Source(value) {
		case agentdiscovery.SourceLocal, agentdiscovery.SourceACPRegistry, agentdiscovery.SourceA2ACatalog:
			sources = append(sources, agentdiscovery.Source(value))
		}
	}
	if len(sources) == 0 {
		return agentcatalog.DefaultSources()
	}
	return sources
}

func authorizeMatrixRuntimeRequest(w http.ResponseWriter, r *http.Request, apiKey string) bool {
	if apiKey == "" {
		return true
	}
	if requestMatrixAPIKey(r) != apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func requestMatrixAPIKey(r *http.Request) string {
	if key := strings.TrimSpace(r.Header.Get("X-Matrix-Key")); key != "" {
		return key
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}

func splitCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func init() {
	rootCmd.AddCommand(runCmd)
}
