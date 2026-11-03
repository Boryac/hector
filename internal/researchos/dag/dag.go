// Package dag 负责工作流的结构校验与拓扑排序。
//
// 设计原则：DAG 的正确性由后端强制校验，绝不信任 Planner 输出的边。
package dag

import (
	"fmt"
	"sort"
	"strings"

	"github.com/verikod/hector/internal/researchos/domain"
)

// Validate 校验节点集合：ID 唯一、依赖存在、无自环、无环。
func Validate(nodes []domain.WorkflowNode) error {
	if len(nodes) == 0 {
		return fmt.Errorf("工作流为空：至少需要一个节点")
	}
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		id := strings.TrimSpace(n.ID)
		if id == "" {
			return fmt.Errorf("存在空节点 ID")
		}
		if ids[id] {
			return fmt.Errorf("节点 ID 重复：%s", id)
		}
		ids[id] = true
	}
	for _, n := range nodes {
		for _, d := range n.DependsOn {
			if d == n.ID {
				return fmt.Errorf("节点 %s 依赖自身", n.ID)
			}
			if !ids[d] {
				return fmt.Errorf("节点 %s 依赖不存在的节点 %s", n.ID, d)
			}
		}
	}
	if _, err := TopoSort(nodes); err != nil {
		return err
	}
	return nil
}

// TopoSort 返回拓扑序的节点 ID（Kahn 算法，同层按原始顺序稳定）。
func TopoSort(nodes []domain.WorkflowNode) ([]string, error) {
	index := make(map[string]int, len(nodes))
	for i, n := range nodes {
		index[n.ID] = i
	}
	indeg := make(map[string]int, len(nodes))
	children := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		indeg[n.ID] += len(n.DependsOn)
		for _, d := range n.DependsOn {
			children[d] = append(children[d], n.ID)
		}
	}
	// 用有序切片模拟优先队列，保证输出稳定。
	ready := make([]string, 0, len(nodes))
	for _, n := range nodes {
		if indeg[n.ID] == 0 {
			ready = append(ready, n.ID)
		}
	}
	sort.SliceStable(ready, func(i, j int) bool { return index[ready[i]] < index[ready[j]] })

	out := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, id)
		next := make([]string, 0, len(children[id]))
		for _, c := range children[id] {
			indeg[c]--
			if indeg[c] == 0 {
				next = append(next, c)
			}
		}
		sort.SliceStable(next, func(i, j int) bool { return index[next[i]] < index[next[j]] })
		ready = append(ready, next...)
	}
	if len(out) != len(nodes) {
		var cycle []string
		for _, n := range nodes {
			if indeg[n.ID] > 0 {
				cycle = append(cycle, n.ID)
			}
		}
		sort.Strings(cycle)
		return nil, fmt.Errorf("工作流存在环，涉及节点：%s", strings.Join(cycle, ", "))
	}
	return out, nil
}

// BuildEdges 由 depends_on 推导边集合（定义层 nodes[] + edges[]）。
func BuildEdges(nodes []domain.WorkflowNode) []domain.Edge {
	edges := make([]domain.Edge, 0, len(nodes))
	for _, n := range nodes {
		for _, d := range n.DependsOn {
			edges = append(edges, domain.Edge{Source: d, Target: n.ID})
		}
	}
	return edges
}

// Ready 判断节点在给定完成集合下是否可执行。
func Ready(n domain.WorkflowNode, completed map[string]bool) bool {
	for _, d := range n.DependsOn {
		if !completed[d] {
			return false
		}
	}
	return true
}

// Depth 计算每个节点的层级（用于画布布局）。
func Depth(nodes []domain.WorkflowNode) map[string]int {
	order, err := TopoSort(nodes)
	if err != nil {
		return map[string]int{}
	}
	byID := make(map[string]domain.WorkflowNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	depth := make(map[string]int, len(nodes))
	for _, id := range order {
		max := 0
		for _, d := range byID[id].DependsOn {
			if depth[d]+1 > max {
				max = depth[d] + 1
			}
		}
		depth[id] = max
	}
	return depth
}
