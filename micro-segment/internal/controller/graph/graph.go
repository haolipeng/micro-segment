// Package graph 提供网络拓扑图数据结构
// 从NeuVector controller/graph/graph.go简化提取
package graph

import (
	"reflect"
	"sync"
)

// NewLinkCallback 新链接回调
type NewLinkCallback func(src, link, dst string)

// DelNodeCallback 删除节点回调
type DelNodeCallback func(node string)

// DelLinkCallback 删除链接回调
type DelLinkCallback func(src, link, dst string)

// UpdateLinkAttrCallback 更新链接属性回调
type UpdateLinkAttrCallback func(src, link, dst string)

// ConnectedNodeCallback 连接节点回调
type ConnectedNodeCallback func(node string) bool

// PurgeOutLinkCallback 清理出链接回调
type PurgeOutLinkCallback func(src, link, dst string, attr interface{}, param interface{}) bool

// graphLink 链接到其他节点
type graphLink struct {
	ends map[string]interface{} // 节点端点名称
}

// graphNode 每个节点有入链接和出链接
type graphNode struct {
	ins  map[string]*graphLink // 入链接
	outs map[string]*graphLink // 出链接
}

// Graph 网络拓扑图
type Graph struct {
	mutex            sync.RWMutex
	nodes            map[string]*graphNode // 节点名称 -> 节点
	cbNewLink        NewLinkCallback
	cbDelNode        DelNodeCallback
	cbDelLink        DelLinkCallback
	cbUpdateLinkAttr UpdateLinkAttrCallback
}

// NewGraph 创建新图
func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*graphNode),
	}
}

// RegisterNewLinkHook 注册新链接钩子
func (g *Graph) RegisterNewLinkHook(cb NewLinkCallback) {
	g.cbNewLink = cb
}

// RegisterDelNodeHook 注册删除节点钩子
func (g *Graph) RegisterDelNodeHook(cb DelNodeCallback) {
	g.cbDelNode = cb
}

// RegisterDelLinkHook 注册删除链接钩子
func (g *Graph) RegisterDelLinkHook(cb DelLinkCallback) {
	g.cbDelLink = cb
}

// RegisterUpdateLinkAttrHook 注册更新链接属性钩子
func (g *Graph) RegisterUpdateLinkAttrHook(cb UpdateLinkAttrCallback) {
	g.cbUpdateLinkAttr = cb
}

// Reset 重置图
func (g *Graph) Reset() {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.nodes = make(map[string]*graphNode)
}

// AddLink 添加链接
func (g *Graph) AddLink(src, link, dst string, attr interface{}) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	var gn *graphNode
	var gl *graphLink
	var ok, newlink, updattr bool

	if gn, ok = g.nodes[src]; !ok {
		gl = &graphLink{ends: make(map[string]interface{})}
		gl.ends[dst] = attr

		gn = &graphNode{
			ins:  make(map[string]*graphLink),
			outs: make(map[string]*graphLink),
		}
		gn.outs[link] = gl

		g.nodes[src] = gn
		newlink = true
	} else if gl, ok = gn.outs[link]; !ok {
		gl = &graphLink{ends: make(map[string]interface{})}
		gl.ends[dst] = attr

		gn.outs[link] = gl
		newlink = true
	} else if _, ok = gl.ends[dst]; !ok {
		gl.ends[dst] = attr
		newlink = true
	} else {
		if !reflect.DeepEqual(gl.ends[dst], attr) {
			gl.ends[dst] = attr
			updattr = true
		}
	}

	if gn, ok = g.nodes[dst]; !ok {
		gl = &graphLink{ends: make(map[string]interface{})}
		gl.ends[src] = attr

		gn = &graphNode{
			ins:  make(map[string]*graphLink),
			outs: make(map[string]*graphLink),
		}
		gn.ins[link] = gl

		g.nodes[dst] = gn
		newlink = true
	} else if gl, ok = gn.ins[link]; !ok {
		gl = &graphLink{ends: make(map[string]interface{})}
		gl.ends[src] = attr

		gn.ins[link] = gl
		newlink = true
	} else if _, ok = gl.ends[src]; !ok {
		gl.ends[src] = attr
		newlink = true
	} else {
		if !reflect.DeepEqual(gl.ends[src], attr) {
			gl.ends[src] = attr
			updattr = true
		}
	}

	if newlink && g.cbNewLink != nil {
		g.cbNewLink(src, link, dst)
	}
	if updattr && g.cbUpdateLinkAttr != nil {
		g.cbUpdateLinkAttr(src, link, dst)
	}
}

// Attr 获取链接属性
func (g *Graph) Attr(src, link, dst string) interface{} {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	if s, ok := g.nodes[src]; ok {
		if gl, ok := s.outs[link]; ok {
			if attr, ok := gl.ends[dst]; ok {
				return attr
			}
		}
	}
	return nil
}

// DeleteLink 删除链接
func (g *Graph) DeleteLink(src, link, dst string) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	var s, d *graphNode
	var ok bool

	if s, ok = g.nodes[src]; !ok {
		return
	}
	if d, ok = g.nodes[dst]; !ok {
		return
	}

	if gl, ok := s.outs[link]; ok {
		if _, ok = gl.ends[dst]; ok {
			delete(gl.ends, dst)
			if len(gl.ends) == 0 {
				delete(s.outs, link)
				if g.cbDelLink != nil {
					g.cbDelLink(src, link, dst)
				}
			}
		}
	}

	if gl, ok := d.ins[link]; ok {
		if _, ok = gl.ends[src]; ok {
			delete(gl.ends, src)
			if len(gl.ends) == 0 {
				delete(d.ins, link)
				if g.cbDelLink != nil {
					g.cbDelLink(src, link, dst)
				}
			}
		}
	}
}

// DeleteNode 删除节点
func (g *Graph) DeleteNode(node string) string {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	var gn *graphNode
	var ok bool

	if gn, ok = g.nodes[node]; !ok {
		return ""
	}

	// 删除所有入链接
	for link, gl := range gn.ins {
		for n := range gl.ends {
			g.deleteLink(n, link, node)
		}
	}

	// 删除所有出链接
	for link, gl := range gn.outs {
		for n := range gl.ends {
			g.deleteLink(node, link, n)
		}
	}

	delete(g.nodes, node)

	if g.cbDelNode != nil {
		g.cbDelNode(node)
	}

	return node
}

// deleteLink 内部删除链接（不加锁）
func (g *Graph) deleteLink(src, link, dst string) {
	var s, d *graphNode
	var ok bool

	if s, ok = g.nodes[src]; !ok {
		return
	}
	if d, ok = g.nodes[dst]; !ok {
		return
	}

	if gl, ok := s.outs[link]; ok {
		if _, ok = gl.ends[dst]; ok {
			delete(gl.ends, dst)
			if len(gl.ends) == 0 {
				delete(s.outs, link)
			}
		}
	}

	if gl, ok := d.ins[link]; ok {
		if _, ok = gl.ends[src]; ok {
			delete(gl.ends, src)
			if len(gl.ends) == 0 {
				delete(d.ins, link)
			}
		}
	}
}

// Node 获取节点
func (g *Graph) Node(v string) string {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	if _, ok := g.nodes[v]; ok {
		return v
	}
	return ""
}

// All 获取所有节点
func (g *Graph) All() []string {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	ret := make([]string, 0, len(g.nodes))
	for v := range g.nodes {
		ret = append(ret, v)
	}
	return ret
}

// Ins 获取节点的所有入节点
func (g *Graph) Ins(node string) []string {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	if _, ok := g.nodes[node]; !ok {
		return nil
	}

	ret := make([]string, 0)
	n := g.nodes[node]
	for _, l := range n.ins {
		for v := range l.ends {
			ret = append(ret, v)
		}
	}
	return ret
}

// Outs 获取节点的所有出节点
func (g *Graph) Outs(node string) []string {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	if _, ok := g.nodes[node]; !ok {
		return nil
	}

	ret := make([]string, 0)
	n := g.nodes[node]
	for _, l := range n.outs {
		for v := range l.ends {
			ret = append(ret, v)
		}
	}
	return ret
}

// GetNodeCount 获取节点数量
func (g *Graph) GetNodeCount() int {
	g.mutex.RLock()
	defer g.mutex.RUnlock()
	return len(g.nodes)
}

// GetLinkCount 获取链接数量
func (g *Graph) GetLinkCount() int {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	count := 0
	for _, n := range g.nodes {
		for _, l := range n.outs {
			count += len(l.ends)
		}
	}
	return count
}
