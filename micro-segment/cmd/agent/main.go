// Package main Agent入口
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/micro-segment/internal/agent/engine"
	"github.com/micro-segment/internal/agent/network"
)

var (
	version   = "0.1.0"
	buildTime = "2026-01-07"
)

func main() {
	// 命令行参数
	var (
		dpSocket     = flag.String("dp-socket", "/var/run/dp.sock", "DP Unix socket path")
		grpcAddr     = flag.String("grpc-addr", "localhost:18400", "Controller gRPC address")
		logLevel     = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		enableCapture = flag.Bool("enable-capture", true, "Enable Docker container traffic capture")
		showVer      = flag.Bool("version", false, "Show version")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("micro-segment agent %s (built %s)\n", version, buildTime)
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

	// 获取主机信息
	hostname, _ := os.Hostname()
	hostID := getHostID()
	agentID := uuid.New().String()

	log.WithFields(log.Fields{
		"version":        version,
		"host_id":        hostID,
		"host_name":      hostname,
		"agent_id":       agentID,
		"enable_capture": *enableCapture,
	}).Info("Starting micro-segment agent")

	// 初始化网络管理器（如果启用流量捕获）
	var networkManager *network.Manager
	if *enableCapture {
		log.Info("Initializing Docker container traffic capture")
		
		networkManager, err = network.NewManager()
		if err != nil {
			log.WithError(err).Fatal("Failed to create network manager")
		}

		// 验证网络设置
		if err := networkManager.ValidateSetup(); err != nil {
			log.WithError(err).Warn("Network setup validation failed, disabling traffic capture")
			networkManager = nil
		} else {
			// 启动网络管理器
			if err := networkManager.Start(); err != nil {
				log.WithError(err).Warn("Failed to start network manager, disabling traffic capture")
				networkManager = nil
			} else {
				log.Info("Docker container traffic capture enabled")
			}
		}
	}

	// 创建引擎配置
	config := &engine.Config{
		AgentID:        agentID,
		HostID:         hostID,
		HostName:       hostname,
		DPSocketPath:   *dpSocket,
		GRPCAddr:       *grpcAddr,
		NetworkManager: networkManager,
	}

	// 创建并启动引擎
	eng := engine.NewEngine(config)
	if err := eng.Start(); err != nil {
		log.WithError(err).Fatal("Failed to start agent engine")
	}

	log.Info("Agent started successfully")

	// 等待退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Info("Shutting down...")
	
	// 停止网络管理器
	if networkManager != nil {
		if err := networkManager.Stop(); err != nil {
			log.WithError(err).Warn("Failed to stop network manager")
		}
	}
	
	eng.Stop()
	log.Info("Agent stopped")
}

// getHostID 获取主机ID
func getHostID() string {
	// 尝试读取machine-id
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		return string(data[:len(data)-1]) // 去掉换行符
	}
	// 回退到UUID
	return uuid.New().String()
}
