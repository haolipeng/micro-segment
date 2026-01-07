// Package main Controller入口
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"

	"github.com/micro-segment/internal/controller/cache"
	ctrlgrpc "github.com/micro-segment/internal/controller/grpc"
	"github.com/micro-segment/internal/controller/policy"
	"github.com/micro-segment/internal/controller/rest"
)

var (
	version   = "0.1.0"
	buildTime = "unknown"
)

func main() {
	// 命令行参数
	var (
		httpPort = flag.Int("http-port", 10443, "HTTP API port")
		grpcPort = flag.Int("grpc-port", 18400, "gRPC port")
		logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		showVer  = flag.Bool("version", false, "Show version")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("micro-segment controller %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	// 设置日志级别
	level, err := log.ParseLevel(*logLevel)
	if err != nil {
		level = log.InfoLevel
	}
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	log.WithFields(log.Fields{
		"version":   version,
		"http_port": *httpPort,
		"grpc_port": *grpcPort,
	}).Info("Starting micro-segment controller")

	// 初始化缓存
	c := cache.NewCache()
	log.Info("Cache initialized")

	// 初始化策略引擎
	p := policy.NewEngine()
	log.Info("Policy engine initialized")

	// 初始化gRPC服务器
	grpcServer := ctrlgrpc.NewServer(*grpcPort, c, p)

	// 设置gRPC回调
	grpcServer.SetOnAgentJoin(func(agentID, hostID string) {
		log.WithFields(log.Fields{
			"agent_id": agentID,
			"host_id":  hostID,
		}).Info("Agent joined")
	})

	grpcServer.SetOnAgentLeave(func(agentID string) {
		log.WithFields(log.Fields{
			"agent_id": agentID,
		}).Info("Agent left")
	})

	// 启动gRPC服务器
	if err := grpcServer.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start gRPC server")
	}
	log.WithField("port", *grpcPort).Info("gRPC server started")

	// 初始化REST路由
	router := rest.NewRouter(c, p)

	// 启动HTTP服务器
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *httpPort),
		Handler: router,
	}

	go func() {
		log.WithField("port", *httpPort).Info("HTTP server started")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("HTTP server error")
		}
	}()

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutting down...")

	// 停止服务
	grpcServer.Stop()
	httpServer.Close()

	log.Info("Controller stopped")
}
