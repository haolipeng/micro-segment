// Package rest 提供REST API处理
// 从NeuVector controller/rest简化提取
package rest

import (
	"encoding/json"
	"net/http"
	"strconv"

	controller "github.com/micro-segment/internal/controller"
	"github.com/micro-segment/internal/controller/cache"
	"github.com/micro-segment/internal/controller/policy"
)

// Handler REST API处理器
type Handler struct {
	cache  *cache.Cache
	policy *policy.Engine
}

// NewHandler 创建处理器
// 初始化REST API处理器和依赖组件
func NewHandler(c *cache.Cache, p *policy.Engine) *Handler {
	return &Handler{
		cache:  c,
		policy: p,
	}
}

// Response API响应
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// writeJSON 写入JSON响应
// 设置Content-Type并编码JSON响应
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeError 写入错误响应
// 返回标准格式的错误响应
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, Response{
		Code:    status,
		Message: message,
	})
}

// writeSuccess 写入成功响应
// 返回标准格式的成功响应
func writeSuccess(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, Response{
		Code: 0,
		Data: data,
	})
}

// --- 工作负载API ---

// ListWorkloads 列出工作负载
// 返回所有工作负载的列表
func (h *Handler) ListWorkloads(w http.ResponseWriter, r *http.Request) {
	workloads := h.cache.ListWorkloads()
	writeSuccess(w, workloads)
}

// GetWorkload 获取工作负载
// 根据ID查询单个工作负载详情
func (h *Handler) GetWorkload(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing workload id")
		return
	}

	wl := h.cache.GetWorkload(id)
	if wl == nil {
		writeError(w, http.StatusNotFound, "workload not found")
		return
	}

	writeSuccess(w, wl)
}

// --- 组API ---

// ListGroups 列出组
// 返回所有安全组的列表
func (h *Handler) ListGroups(w http.ResponseWriter, r *http.Request) {
	groups := h.cache.ListGroups()
	writeSuccess(w, groups)
}

// GetGroup 获取组
// 根据名称查询单个安全组详情
func (h *Handler) GetGroup(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing group name")
		return
	}

	group := h.cache.GetGroup(name)
	if group == nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}

	writeSuccess(w, group)
}

// CreateGroup 创建组
// 创建新的安全组
func (h *Handler) CreateGroup(w http.ResponseWriter, r *http.Request) {
	var group controller.Group
	if err := json.NewDecoder(r.Body).Decode(&group); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if group.Name == "" {
		writeError(w, http.StatusBadRequest, "missing group name")
		return
	}

	h.cache.AddGroup(&group)
	writeSuccess(w, group)
}

// DeleteGroup 删除组
// 根据名称删除安全组
func (h *Handler) DeleteGroup(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing group name")
		return
	}

	h.cache.DeleteGroup(name)
	writeSuccess(w, nil)
}

// --- 策略API ---

// ListPolicies 列出策略
// 返回所有网络策略规则列表
func (h *Handler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	rules := h.policy.ListRules()
	writeSuccess(w, rules)
}

// GetPolicy 获取策略
// 根据ID查询单个策略规则详情
func (h *Handler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing policy id")
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid policy id")
		return
	}

	rule := h.policy.GetRule(uint32(id))
	if rule == nil {
		writeError(w, http.StatusNotFound, "policy not found")
		return
	}

	writeSuccess(w, rule)
}

// CreatePolicy 创建策略
// 创建新的网络策略规则
func (h *Handler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	var rule controller.PolicyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.policy.AddRule(&rule); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeSuccess(w, rule)
}

// UpdatePolicy 更新策略
// 更新现有的网络策略规则
func (h *Handler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	var rule controller.PolicyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.policy.UpdateRule(&rule); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, rule)
}

// DeletePolicy 删除策略
func (h *Handler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing policy id")
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid policy id")
		return
	}

	if err := h.policy.DeleteRule(uint32(id)); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeSuccess(w, nil)
}

// --- 网络拓扑API ---

// GetNetworkGraph 获取网络拓扑图
func (h *Handler) GetNetworkGraph(w http.ResponseWriter, r *http.Request) {
	graph := h.cache.GetNetworkGraph()
	writeSuccess(w, graph)
}

// --- 主机API ---

// ListHosts 列出主机
func (h *Handler) ListHosts(w http.ResponseWriter, r *http.Request) {
	hosts := h.cache.ListHosts()
	writeSuccess(w, hosts)
}

// --- Agent API ---

// ListAgents 列出Agent
func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents := h.cache.ListAgents()
	writeSuccess(w, agents)
}

// --- 统计API ---

// GetStats 获取统计信息
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"workloads":   len(h.cache.ListWorkloads()),
		"groups":      len(h.cache.ListGroups()),
		"policies":    h.policy.GetRuleCount(),
		"hosts":       len(h.cache.ListHosts()),
		"agents":      len(h.cache.ListAgents()),
		"graph_nodes": h.cache.GetGraphNodeCount(),
		"graph_links": h.cache.GetGraphLinkCount(),
	}
	writeSuccess(w, stats)
}
