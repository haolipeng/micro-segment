// Package grpc 提供gRPC服务
// 实现 ControllerService
package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	pb "github.com/micro-segment/api/proto"
	"github.com/micro-segment/internal/controller/cache"
	"github.com/micro-segment/internal/controller/policy"
)

// Server gRPC服务器
type Server struct {
	pb.UnimplementedControllerServiceServer

	mutex      sync.RWMutex
	listener   net.Listener
	grpcServer *grpc.Server
	port       int
	running    bool

	// 依赖
	cache  *cache.Cache
	policy *policy.Engine

	// Agent管理
	agents map[string]*AgentState

	// 回调函数
	onAgentJoin  func(agentID, hostID string)
	onAgentLeave func(agentID string)
}

// AgentState Agent状态
type AgentState struct {
	Info       *pb.AgentInfo
	LastSeen   time.Time
	Online     bool
	Stats      *pb.AgentStats
}

// NewServer 创建gRPC服务器
// 初始化服务器配置和Agent状态管理
func NewServer(port int, c *cache.Cache, p *policy.Engine) *Server {
	return &Server{
		port:   port,
		cache:  c,
		policy: p,
		agents: make(map[string]*AgentState),
	}
}

// SetOnAgentJoin 设置Agent加入回调
// 注册Agent连接事件处理函数
func (s *Server) SetOnAgentJoin(cb func(agentID, hostID string)) {
	s.onAgentJoin = cb
}

// SetOnAgentLeave 设置Agent离开回调
// 注册Agent断开连接事件处理函数
func (s *Server) SetOnAgentLeave(cb func(agentID string)) {
	s.onAgentLeave = cb
}

// Start 启动服务器
// 启动gRPC服务监听和Agent超时检测
func (s *Server) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	var err error
	s.listener, err = net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	s.grpcServer = grpc.NewServer()
	pb.RegisterControllerServiceServer(s.grpcServer, s)

	s.running = true

	go func() {
		if err := s.grpcServer.Serve(s.listener); err != nil {
			// 日志记录错误
		}
	}()

	// 启动Agent超时检测
	go s.agentTimeoutChecker()

	return nil
}

// Stop 停止服务器
// 优雅关闭gRPC服务器和监听器
func (s *Server) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return
	}

	s.grpcServer.GracefulStop()
	s.listener.Close()
	s.running = false
}

// IsRunning 检查服务器是否运行中
// 线程安全地返回服务器运行状态
func (s *Server) IsRunning() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.running
}

// agentTimeoutChecker 检测Agent超时
// 定期检查Agent心跳超时并标记离线
func (s *Server) agentTimeoutChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for s.running {
		<-ticker.C
		s.checkAgentTimeout()
	}
}

// checkAgentTimeout 检查Agent超时
// 检查所有Agent的最后心跳时间并更新状态
func (s *Server) checkAgentTimeout() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	timeout := 60 * time.Second
	now := time.Now()

	for agentID, state := range s.agents {
		if state.Online && now.Sub(state.LastSeen) > timeout {
			state.Online = false
			if s.onAgentLeave != nil {
				go s.onAgentLeave(agentID)
			}
		}
	}
}

// ============================================
// ControllerService 实现
// ============================================

// Register Agent注册
// 处理Agent注册请求并返回集群配置
func (s *Server) Register(ctx context.Context, req *pb.AgentInfo) (*pb.RegisterResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.agents[req.AgentId] = &AgentState{
		Info:     req,
		LastSeen: time.Now(),
		Online:   true,
	}

	if s.onAgentJoin != nil {
		go s.onAgentJoin(req.AgentId, req.HostId)
	}

	return &pb.RegisterResponse{
		Code:           0,
		Message:        "registered",
		ClusterId:      "micro-segment-cluster",
		ReportInterval: 5,
	}, nil
}

// Heartbeat Agent心跳
// 处理Agent心跳请求并更新在线状态
func (s *Server) Heartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if state, ok := s.agents[req.AgentId]; ok {
		state.LastSeen = time.Now()
		state.Online = true
		state.Stats = req.Stats
	}

	return &pb.HeartbeatResponse{
		Code:      0,
		Timestamp: uint64(time.Now().Unix()),
	}, nil
}

// ReportConnections 上报连接
// 接收Agent上报的网络连接数据并更新缓存
func (s *Server) ReportConnections(ctx context.Context, req *pb.ConnectionReport) (*pb.ReportResponse, error) {
	// 处理连接上报
	for _, conn := range req.Connections {
		s.cache.UpdateConnectionFromProto(conn)
	}

	return &pb.ReportResponse{
		Code:           0,
		Message:        "ok",
		ReportInterval: 5,
	}, nil
}

// ReportThreats 上报威胁日志
// 接收Agent上报的安全威胁检测结果
func (s *Server) ReportThreats(ctx context.Context, req *pb.ThreatReport) (*pb.ReportResponse, error) {
	// 处理威胁日志
	// TODO: 存储威胁日志

	return &pb.ReportResponse{
		Code:    0,
		Message: "ok",
	}, nil
}

// ReportWorkload 上报工作负载变更
// 处理容器生命周期事件并更新工作负载缓存
func (s *Server) ReportWorkload(ctx context.Context, req *pb.WorkloadEvent) (*pb.ReportResponse, error) {
	switch req.EventType {
	case "add", "update":
		s.cache.UpdateWorkloadFromProto(req.Workload)
	case "delete":
		if req.Workload != nil {
			s.cache.DeleteWorkload(req.Workload.Id)
		}
	}

	return &pb.ReportResponse{
		Code:    0,
		Message: "ok",
	}, nil
}

// GetPolicies 获取策略
// 返回指定工作负载的网络策略规则列表
func (s *Server) GetPolicies(ctx context.Context, req *pb.PolicyRequest) (*pb.PolicyList, error) {
	rules := s.policy.ListRules()

	pbRules := make([]*pb.PolicyRule, 0, len(rules))
	for _, rule := range rules {
		pbRules = append(pbRules, &pb.PolicyRule{
			Id:       rule.ID,
			From:     rule.From,
			To:       rule.To,
			Ports:    rule.Ports,
			Action:   actionToProto(rule.Action),
			Priority: rule.Priority,
			Disable:  rule.Disable,
			Comment:  rule.Comment,
		})
	}

	return &pb.PolicyList{
		Rules: pbRules,
	}, nil
}

// actionToProto 转换动作到proto
// 将策略动作字符串转换为protobuf枚举值
func actionToProto(action string) uint32 {
	switch action {
	case "open":
		return 0
	case "allow":
		return 1
	case "deny":
		return 2
	case "violate":
		return 3
	default:
		return 3
	}
}

// GetAgentCount 获取Agent数量
// 返回已注册的Agent总数
func (s *Server) GetAgentCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return len(s.agents)
}

// GetOnlineAgentCount 获取在线Agent数量
// 返回当前在线的Agent数量
func (s *Server) GetOnlineAgentCount() int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	count := 0
	for _, state := range s.agents {
		if state.Online {
			count++
		}
	}
	return count
}
