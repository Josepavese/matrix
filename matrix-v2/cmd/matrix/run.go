package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jose/matrix-v2/internal/logic/channelcfg"
	"github.com/jose/matrix-v2/internal/logic/daemon"
	"github.com/jose/matrix-v2/internal/logic/logging"
	"github.com/jose/matrix-v2/internal/logic/onboarding"
	"github.com/jose/matrix-v2/internal/logic/runtimecheck"
	"github.com/jose/matrix-v2/internal/logic/session"
	"github.com/jose/matrix-v2/internal/logic/system_tools"
	"github.com/jose/matrix-v2/internal/providers/acp"
	"github.com/jose/matrix-v2/internal/providers/agents"
	"github.com/jose/matrix-v2/internal/providers/osfs"
	"github.com/jose/matrix-v2/internal/providers/oslog"
	"github.com/jose/matrix-v2/internal/providers/telegram"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the Matrix V2 background daemon",
	Run: func(cmd *cobra.Command, args []string) {
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
		wizard := onboarding.NewWizard(onboarding.WizardDependencies{
			Storage:   d.App.Store,
			Config:    d.ConfigMgr,
			Localizer: localizer,
			Proc:      d.ExecProv,
			Installer: d.Installer,
			RegClient: d.Installer.RegistryClient(),
			FS:        d.App.FS,
			Net:       d.NetProv,
		})
		agentRouter := agents.NewRouter(d.Supervisor)
		agentRouter.SetTrustMode(func() bool {
			return d.App.Config.GetWithDefault("agent.trust_mode", "true") == "true"
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
		if agent := d.App.Config.GetWithDefault("default_agent", ""); agent != "" {
			sessionMgr.SetDefaultAgent(agent)
		}
		if agent := d.App.Config.GetWithDefault("action_agent", ""); agent != "" {
			sessionMgr.SetActionAgent(agent)
		}

		// Configurable addresses with defaults
		jsonrpcAddr := d.App.Config.GetWithDefault("jsonrpc_addr", DefaultJSONRPCAddr)
		acpHTTPAddr := d.App.Config.GetWithDefault("acp_http_addr", DefaultACPHTTPAddr)

		// ACP REST Server
		acpServer := acp.NewServer(sessionMgr)
		if apiKey := d.App.Config.GetWithDefault("acp_api_key", ""); apiKey != "" {
			acpServer.WithAPIKey(apiKey)
			log.Info("acp api key configured", "event", "acp_apikey_set")
		}
		mux := http.NewServeMux()
		acpServer.RegisterRoutes(mux)
		mux.HandleFunc("/_matrix/runtime", func(w http.ResponseWriter, r *http.Request) {
			report, err := runtimecheck.BuildReport(runtimecheck.BuildInput{
				Store:         d.App.Store,
				Registry:      d.Registry,
				Process:       d.ExecProv,
				ConfigManager: d.App.Config,
				ConfigReader:  d.ConfigMgr,
				Net:           d.NetProv,
				JSONRPCAddr:   jsonrpcAddr,
				ACPHTTPAddr:   acpHTTPAddr,
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
			if daemonAPIKey := d.App.Config.GetWithDefault("daemon_api_key", ""); daemonAPIKey != "" {
				srv.WithAPIKey(daemonAPIKey)
				log.Info("daemon API key configured", "event", "daemon_apikey_set")
			}

		// Telegram Bot (conditional)
		tgCfg, tgSource, err := channelcfg.LoadTelegramConfig(d.ConfigMgr, d.App.Config)
		if err == nil && tgCfg.Enabled && tgCfg.Token != "" {
			log.Info("telegram configuration loaded", "event", "telegram_config_loaded", "source", tgSource)
			tgBot, err := telegram.NewBot(tgCfg.Token, sessionMgr)
			if err != nil {
				log.Error("telegram bot initialization failed", "event", "telegram_init_failed", "error", err)
			} else {
				go func() {
					if err := tgBot.Start(ctx); err != nil {
						log.Error("telegram gateway stopped with error", "event", "telegram_stopped", "error", err)
					}
				}()
			}
		}

		// Start ACP HTTP Server
		httpServer := &http.Server{Addr: acpHTTPAddr, Handler: mux}
		go func() {
			log.Info("starting acp http server", "event", "acp_http_starting", "addr", acpHTTPAddr)
			if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
				log.Error("acp http server stopped with error", "event", "acp_http_stopped", "error", err, "addr", acpHTTPAddr)
			}
		}()

		// Graceful shutdown: wait for signal then shut down HTTP server + agent router
		go func() {
			<-ctx.Done()
			log.Info("shutting down...", "event", "shutdown_started")
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
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
}
