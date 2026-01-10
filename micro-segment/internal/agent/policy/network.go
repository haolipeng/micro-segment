// Package policy 提供网络策略管理
// 从NeuVector agent/policy简化提取
package policy

import (
	"net"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/micro-segment/internal/agent"
	"github.com/micro-segment/internal/agent/dp"
)

// NetworkPolicy 网络策略管理器
type NetworkPolicy struct {
	mutex    sync.RWMutex
	rules    map[uint32]*agent.PolicyRule
	dpClient *dp.DPClient
}

// NewNetworkPolicy 创建网络策略管理器
// 初始化策略规则存储和DP客户端连接
func NewNetworkPolicy(dpClient *dp.DPClient) *NetworkPolicy {
	return &NetworkPolicy{
		rules:    make(map[uint32]*agent.PolicyRule),
		dpClient: dpClient,
	}
}

// AddRule 添加规则
// 添加单条网络策略规则到内存
func (p *NetworkPolicy) AddRule(rule *agent.PolicyRule) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.rules[rule.ID] = rule
	log.WithFields(log.Fields{
		"id":     rule.ID,
		"from":   rule.From,
		"to":     rule.To,
		"action": rule.Action,
	}).Debug("Policy rule added")
}

// DeleteRule 删除规则
// 从内存中移除指定ID的策略规则
func (p *NetworkPolicy) DeleteRule(id uint32) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	delete(p.rules, id)
	log.WithField("id", id).Debug("Policy rule deleted")
}

// GetRule 获取规则
// 根据ID查找并返回策略规则
func (p *NetworkPolicy) GetRule(id uint32) *agent.PolicyRule {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.rules[id]
}

// ListRules 列出所有规则
// 返回当前所有策略规则的副本
func (p *NetworkPolicy) ListRules() []*agent.PolicyRule {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	result := make([]*agent.PolicyRule, 0, len(p.rules))
	for _, rule := range p.rules {
		result = append(result, rule)
	}
	return result
}

// UpdateRules 批量更新规则
// 替换所有策略规则并同步到DP层
func (p *NetworkPolicy) UpdateRules(rules []*agent.PolicyRule) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// 清空旧规则
	p.rules = make(map[uint32]*agent.PolicyRule)

	// 添加新规则
	for _, rule := range rules {
		p.rules[rule.ID] = rule
	}

	log.WithField("count", len(rules)).Info("Policy rules updated")

	// 同步到DP
	p.syncToDP()
}

// syncToDP 同步策略到DP层
// 将内存中的策略规则转换并发送到DP执行
func (p *NetworkPolicy) syncToDP() {
	if p.dpClient == nil || !p.dpClient.IsConnected() {
		return
	}

	dpPolicies := make([]*dp.DPPolicy, 0, len(p.rules))
	for _, rule := range p.rules {
		dpPolicy := p.ruleToDPPolicy(rule)
		if dpPolicy != nil {
			dpPolicies = append(dpPolicies, dpPolicy)
		}
	}

	if err := p.dpClient.SendPolicy(dpPolicies); err != nil {
		log.WithError(err).Error("Failed to sync policies to DP")
	}
}

// ruleToDPPolicy 转换规则为DP策略
// 将Agent策略规则转换为DP层可执行格式
func (p *NetworkPolicy) ruleToDPPolicy(rule *agent.PolicyRule) *dp.DPPolicy {
	// 简化实现：只处理基本的IP/端口规则
	return &dp.DPPolicy{
		ID:      rule.ID,
		Action:  uint8(rule.Action),
		Ingress: rule.Ingress,
	}
}

// MatchPolicy 匹配策略
// 根据网络五元组查找匹配的策略规则
func (p *NetworkPolicy) MatchPolicy(srcIP, dstIP net.IP, dstPort uint16, proto uint8) (uint32, agent.PolicyAction) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	// 简化实现：遍历所有规则进行匹配
	for _, rule := range p.rules {
		// TODO: 实现完整的匹配逻辑
		_ = rule
	}

	// 默认返回Monitor模式下的Violate动作
	return 0, agent.PolicyActionViolate
}

// GetRuleCount 获取规则数量
// 返回当前策略规则总数
func (p *NetworkPolicy) GetRuleCount() int {
	p.mutex.RLock()
	defer p.mutex.RUnlock()
	return len(p.rules)
}
