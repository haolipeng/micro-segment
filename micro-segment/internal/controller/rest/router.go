// Package rest 提供REST API路由
package rest

import (
	"net/http"

	"github.com/micro-segment/internal/controller/cache"
	"github.com/micro-segment/internal/controller/policy"
)

// Router REST API路由器
type Router struct {
	handler *Handler
	mux     *http.ServeMux
}

// NewRouter 创建路由器
func NewRouter(c *cache.Cache, p *policy.Engine) *Router {
	r := &Router{
		handler: NewHandler(c, p),
		mux:     http.NewServeMux(),
	}
	r.setupRoutes()
	return r
}

// setupRoutes 设置路由
func (r *Router) setupRoutes() {
	// 工作负载
	r.mux.HandleFunc("/api/v1/workloads", r.handleWorkloads)
	r.mux.HandleFunc("/api/v1/workload", r.handleWorkload)

	// 组
	r.mux.HandleFunc("/api/v1/groups", r.handleGroups)
	r.mux.HandleFunc("/api/v1/group", r.handleGroup)

	// 策略
	r.mux.HandleFunc("/api/v1/policies", r.handlePolicies)
	r.mux.HandleFunc("/api/v1/policy", r.handlePolicy)

	// 网络拓扑
	r.mux.HandleFunc("/api/v1/graph", r.handleGraph)

	// 主机
	r.mux.HandleFunc("/api/v1/hosts", r.handleHosts)

	// Agent
	r.mux.HandleFunc("/api/v1/agents", r.handleAgents)

	// 统计
	r.mux.HandleFunc("/api/v1/stats", r.handleStats)

	// 健康检查
	r.mux.HandleFunc("/health", r.handleHealth)
}

// ServeHTTP 实现http.Handler接口
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if req.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	r.mux.ServeHTTP(w, req)
}

// handleWorkloads 处理工作负载列表
func (r *Router) handleWorkloads(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.ListWorkloads(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleWorkload 处理单个工作负载
func (r *Router) handleWorkload(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.GetWorkload(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGroups 处理组列表
func (r *Router) handleGroups(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.ListGroups(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGroup 处理单个组
func (r *Router) handleGroup(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.GetGroup(w, req)
	case http.MethodPost:
		r.handler.CreateGroup(w, req)
	case http.MethodDelete:
		r.handler.DeleteGroup(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePolicies 处理策略列表
func (r *Router) handlePolicies(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.ListPolicies(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePolicy 处理单个策略
func (r *Router) handlePolicy(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.GetPolicy(w, req)
	case http.MethodPost:
		r.handler.CreatePolicy(w, req)
	case http.MethodPut:
		r.handler.UpdatePolicy(w, req)
	case http.MethodDelete:
		r.handler.DeletePolicy(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGraph 处理网络拓扑图
func (r *Router) handleGraph(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.GetNetworkGraph(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHosts 处理主机列表
func (r *Router) handleHosts(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.ListHosts(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAgents 处理Agent列表
func (r *Router) handleAgents(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.ListAgents(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStats 处理统计信息
func (r *Router) handleStats(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handler.GetStats(w, req)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleHealth 处理健康检查
func (r *Router) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
