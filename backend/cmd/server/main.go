package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ackwrap/ackrun/internal/application"
	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
)

const defaultListenAddr = "0.0.0.0:8080"

type serverConfig struct {
	ListenAddr string
	APIToken   string
}

func loadServerConfig() (serverConfig, error) {
	config := serverConfig{
		ListenAddr: strings.TrimSpace(os.Getenv("ACKWRAP_LISTEN_ADDR")),
		APIToken:   strings.TrimSpace(os.Getenv("ACKWRAP_API_TOKEN")),
	}
	if config.ListenAddr == "" {
		config.ListenAddr = defaultListenAddr
	}

	host, port, err := net.SplitHostPort(config.ListenAddr)
	if err != nil || port == "" {
		return serverConfig{}, fmt.Errorf("ACKWRAP_LISTEN_ADDR 必须是 host:port 格式: %q", config.ListenAddr)
	}
	if host == "localhost" {
		return config, nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return serverConfig{}, fmt.Errorf("ACKWRAP_LISTEN_ADDR 主机必须是 IP 地址或 localhost: %q", host)
	}
	if !ip.IsLoopback() && config.APIToken == "" {
		return serverConfig{}, errors.New("非回环地址监听必须设置 ACKWRAP_API_TOKEN")
	}
	return config, nil
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "network-repair" {
		if len(os.Args) != 2 {
			fmt.Fprintln(os.Stderr, "网络修复失败：network-repair 不接受额外参数")
			os.Exit(1)
		}
		message, err := service.RepairNetwork(paths.Default())
		if err != nil {
			if errors.Is(err, service.ErrNetworkRepairCoreRunning) {
				fmt.Fprintf(os.Stderr, "网络修复失败：%v\n", err)
				os.Exit(2)
			} else {
				fmt.Fprintln(os.Stderr, "网络修复失败：未能安全恢复 Ackwrap 网络状态，请检查系统日志。")
			}
			os.Exit(1)
		}
		fmt.Println(message)
		return
	}
	if err := run(); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}

func run() error {
	serverCfg, err := loadServerConfig()
	if err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	p := paths.Default()
	app, err := application.New(application.Options{Paths: p, APIToken: serverCfg.APIToken})
	if err != nil {
		return fmt.Errorf("start application: %w", err)
	}
	defer func() {
		if closeErr := app.Close(); closeErr != nil {
			logging.Error("main", "close application failed: %v", closeErr)
		}
	}()
	if err := app.Start(); err != nil {
		return fmt.Errorf("start application schedulers: %w", err)
	}

	server := &http.Server{
		Addr:              serverCfg.ListenAddr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	serverErrors, err := startHTTPServerAndCore(
		server,
		app.RestoreCoreAfterUpdate,
		app.StartCoreIfConfigured,
	)
	if err != nil {
		return fmt.Errorf("listen server: %w", err)
	}

	shutdownSignals := make(chan os.Signal, 1)
	signal.Notify(shutdownSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(shutdownSignals)
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	case shutdownSignal := <-shutdownSignals:
		logging.Info("main", "shutdown signal received: %s", shutdownSignal)
	}
	app.PrepareShutdown()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}
	return nil
}

func startHTTPServerAndCore(server *http.Server, restoreCore func(), startCore func() error) (<-chan error, error) {
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return nil, err
	}
	serverErrors := make(chan error, 1)
	go func() {
		logging.Info("main", "starting server on %s", server.Addr)
		serverErrors <- server.Serve(listener)
	}()
	go func() {
		restoreCore()
		if err := startCore(); err != nil {
			logging.Error("core.auto_start", "start sing-box after Ackwrap startup failed: %v", err)
		}
	}()
	return serverErrors, nil
}
