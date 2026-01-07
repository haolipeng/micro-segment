// Package policy 提供策略管理功能
// 从NeuVector controller/cache/policy.go简化提取
package policy

import (
	"fmt"
	"sort"
	"sync"
	"time"

	controller "github.com/micro-segment/internal/controller"
)

// Engine 策略引擎
type Engine struct {
	mutex sync.RWMutex

	// 规则映射 ID -> Rule
	rules map[uint32]*controller.PolicyRule

	// 规则顺序
	ruleOrder []uint32

	// 组策略模式
	groupModes map[string]controller.PolicyMode
}

// NewEngine 创建策略引擎
func NewEngine() *Engine {
	return &Engine{
		rules:      make(map[uint32]*controller.PolicyRule),
		ruleOrder:  make([]uint32, 0),
		groupModes: make(map[string]controller.PolicyMode),
	}
}

// AddRule 添加规则
func (e *Engine) AddRule(rule *controller.PolicyRule) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if rule.ID == 0 {
		return fmt.Errorf("rule ID cannot be 0")
	}

	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()

	e.rules[rule.ID] = rule

	// 更新规则顺序
	e.updateRuleOrder()

	return nil
}

// UpdateRule 更新规则
func (e *Engine) UpdateRule(rule *controller.PolicyRule) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if _, ok := e.rules[rule.ID]; !ok {
		return fmt.Errorf("rule %d not found", rule.ID)
	}

	rule.UpdatedAt = time.Now()
	e.rules[rule.ID] = rule

	return nil
}

// DeleteRule 删除规则
func (e *Engine) DeleteRule(id uint32) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if _, ok := e.rules[id]; !ok {
		return fmt.Errorf("rule %d not found", id)
	}

	delete(e.rules, id)
	e.updateRuleOrder()

	return nil
}

// GetRule 获取规则
func (e *Engine) GetRule(id uint32) *controller.PolicyRule {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.rules[id]
}

// ListRules 列出所有规则
func (e *Engine) ListRules() []*controller.PolicyRule {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	result := make([]*controller.PolicyRule, 0, len(e.ruleOrder))
	for _, id := range e.ruleOrder {
		if rule, ok := e.rules[id]; ok {
			result = append(result, rule)
		}
	}
	return result
}

// updateRuleOrder 更新规则顺序
func (e *Engine) updateRuleOrder() {
	e.ruleOrder = make([]uint32, 0, len(e.rules))
	for id := range e.rules {
		e.ruleOrder = append(e.ruleOrder, id)
	}

	// 按优先级排序
	sort.Slice(e.ruleOrder, func(i, j int) bool {
		ri := e.rules[e.ruleOrder[i]]
		rj := e.rules[e.ruleOrder[j]]
		return ri.Priority < rj.Priority
	})
}

// SetGroupMode 设置组策略模式
func (e *Engine) SetGroupMode(groupName string, mode controller.PolicyMode) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.groupModes[groupName] = mode
}

// GetGroupMode 获取组策略模式
func (e *Engine) GetGroupMode(groupName string) controller.PolicyMode {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	if mode, ok := e.groupModes[groupName]; ok {
		return mode
	}
	return controller.PolicyModeMonitor // 默认Monitor模式
}

// MatchPolicy 匹配策略
// 返回匹配的规则ID和动作
func (e *Engine) MatchPolicy(from, to string, port uint16, proto uint8, app uint32) (uint32, controller.PolicyAction) {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	for _, id := range e.ruleOrder {
		rule := e.rules[id]
		if rule.Disable {
			continue
		}

		// 检查From匹配
		if rule.From != from && rule.From != "any" {
			continue
		}

		// 检查To匹配
		if rule.To != to && rule.To != "any" {
			continue
		}

		// 检查端口匹配
		if rule.Ports != "" && rule.Ports != "any" {
			if !e.matchPort(rule.Ports, port, proto) {
				continue
			}
		}

		// 检查应用匹配
		if len(rule.Applications) > 0 {
			if !e.matchApp(rule.Applications, app) {
				continue
			}
		}

		// 匹配成功，返回动作
		return rule.ID, e.actionFromString(rule.Action)
	}

	// 没有匹配的规则，使用默认动作
	return 0, e.getDefaultAction(to)
}

// matchPort 匹配端口
func (e *Engine) matchPort(ports string, port uint16, proto uint8) bool {
	// 简化实现：只检查"any"
	if ports == "any" {
		return true
	}
	// TODO: 实现完整的端口匹配逻辑
	return true
}

// matchApp 匹配应用
func (e *Engine) matchApp(apps []uint32, app uint32) bool {
	for _, a := range apps {
		if a == app || a == 0 { // 0表示any
			return true
		}
	}
	return false
}

// actionFromString 从字符串转换动作
func (e *Engine) actionFromString(action string) controller.PolicyAction {
	switch action {
	case "allow":
		return controller.PolicyActionAllow
	case "deny":
		return controller.PolicyActionDeny
	default:
		return controller.PolicyActionViolate
	}
}

// getDefaultAction 获取默认动作
func (e *Engine) getDefaultAction(groupName string) controller.PolicyAction {
	mode := e.GetGroupMode(groupName)
	switch mode {
	case controller.PolicyModeProtect:
		return controller.PolicyActionDeny
	default:
		return controller.PolicyActionViolate
	}
}

// GetRuleCount 获取规则数量
func (e *Engine) GetRuleCount() int {
	e.mutex.RLock()
	defer e.mutex.RUnlock()
	return len(e.rules)
}

// GetEnabledRuleCount 获取启用的规则数量
func (e *Engine) GetEnabledRuleCount() int {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	count := 0
	for _, rule := range e.rules {
		if !rule.Disable {
			count++
		}
	}
	return count
}
