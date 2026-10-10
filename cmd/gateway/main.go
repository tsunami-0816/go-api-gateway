// Package main is the entry point of the Go API Gateway.
//
// It initializes the application context, loads configuration,
// and starts both the data-plane and admin-plane HTTP servers
// following the 9-step startup sequence defined in the architecture.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-api-gateway/gateway/internal/app"
	"github.com/go-api-gateway/gateway/internal/config"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "config/default.yaml", "path to config file")
	healthcheck := flag.Bool("healthcheck", false, "run healthcheck and exit (for Docker HEALTHCHECK)")
	flag.Parse()

	if *healthcheck {
		runHealthcheck()
		return
	}

	// ① 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 创建应用上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 初始化应用（②~⑧ 步在 app.New 中完成）
	application, err := app.New(ctx, cfg, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init app: %v\n", err)
		os.Exit(1)
	}

	// ⑨ 启动数据面（最后启动）
	if err := application.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start app: %v\n", err)
		os.Exit(1)
	}

	// 等待信号优雅关闭
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nshutting down gateway...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout())
	defer shutdownCancel()

	if err := application.Stop(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "error during shutdown: %v\n", err)
	}
	fmt.Println("gateway stopped")
}

// runHealthcheck performs a simple HTTP healthcheck against the admin port.
// Used by Docker HEALTHCHECK in exec form (P2-10).
func runHealthcheck() {
	cfg, err := config.Load("config/default.yaml")
	if err != nil {
		os.Exit(1)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", cfg.Server.Admin.Port)
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil || resp.StatusCode != 200 {
		os.Exit(1)
	}
	resp.Body.Close()
	os.Exit(0)
}
