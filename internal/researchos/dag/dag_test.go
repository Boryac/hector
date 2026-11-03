package dag

import (
	"strings"
	"testing"

	"github.com/verikod/hector/internal/researchos/domain"
)

func node(id string, deps ...string) domain.WorkflowNode {
	return domain.WorkflowNode{ID: id, Type: domain.NodeTypeAnalyze, DependsOn: deps}
}

func TestValidateOK(t *testing.T) {
	nodes := []domain.WorkflowNode{
		node("search-1"),
		node("analyze-1", "search-1"),
		node("verify", "analyze-1"),
	}
	if err := Validate(nodes); err != nil {
		t.Fatalf("合法 DAG 不应报错：%v", err)
	}
}

func TestValidateDuplicateID(t *testing.T) {
	nodes := []domain.WorkflowNode{node("a"), node("a")}
	err := Validate(nodes)
	if err == nil || !strings.Contains(err.Error(), "重复") {
		t.Fatalf("重复 ID 应被拒绝，实际：%v", err)
	}
}

func TestValidateEmptyID(t *testing.T) {
	if err := Validate([]domain.WorkflowNode{node("")}); err == nil {
		t.Fatal("空 ID 应被拒绝")
	}
}

func TestValidateMissingDependency(t *testing.T) {
	err := Validate([]domain.WorkflowNode{node("a", "ghost")})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("缺失依赖应被拒绝并指出节点名，实际：%v", err)
	}
}

func TestValidateSelfCycle(t *testing.T) {
	if err := Validate([]domain.WorkflowNode{node("a", "a")}); err == nil {
		t.Fatal("自环应被拒绝")
	}
}

func TestValidateCycle(t *testing.T) {
	nodes := []domain.WorkflowNode{
		node("a", "c"),
		node("b", "a"),
		node("c", "b"),
	}
	err := Validate(nodes)
	if err == nil || !strings.Contains(err.Error(), "环") {
		t.Fatalf("有环 DAG 应被拒绝，实际：%v", err)
	}
}

func TestTopoSortOrder(t *testing.T) {
	nodes := []domain.WorkflowNode{
		node("c", "b"),
		node("b", "a"),
		node("a"),
	}
	order, err := TopoSort(nodes)
	if err != nil {
		t.Fatalf("拓扑排序失败：%v", err)
	}
	if len(order) != 3 {
		t.Fatalf("排序结果长度应为 3，实际 %d", len(order))
	}
	pos := map[string]int{}
	for i, id := range order {
		pos[id] = i
	}
	if !(pos["a"] < pos["b"] && pos["b"] < pos["c"]) {
		t.Fatalf("拓扑顺序错误：%v", order)
	}
}

func TestTopoSortStableForIndependentNodes(t *testing.T) {
	nodes := []domain.WorkflowNode{node("x"), node("y"), node("z")}
	order, err := TopoSort(nodes)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"x", "y", "z"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("无依赖节点应保持输入顺序，得到 %v", order)
		}
	}
}

func TestBuildEdges(t *testing.T) {
	nodes := []domain.WorkflowNode{node("a"), node("b", "a"), node("c", "a", "b")}
	edges := BuildEdges(nodes)
	if len(edges) != 3 {
		t.Fatalf("边数应为 3，实际 %d", len(edges))
	}
	seen := map[string]bool{}
	for _, e := range edges {
		seen[e.Source+"->"+e.Target] = true
	}
	for _, want := range []string{"a->b", "a->c", "b->c"} {
		if !seen[want] {
			t.Fatalf("缺少边 %s，实际 %v", want, seen)
		}
	}
}

func TestReady(t *testing.T) {
	n := node("b", "a")
	if Ready(n, map[string]bool{}) {
		t.Fatal("依赖未完成时不应就绪")
	}
	if !Ready(n, map[string]bool{"a": true}) {
		t.Fatal("依赖完成后应就绪")
	}
	if !Ready(node("a"), map[string]bool{}) {
		t.Fatal("无依赖节点应始终就绪")
	}
}

func TestDepth(t *testing.T) {
	nodes := []domain.WorkflowNode{node("a"), node("b", "a"), node("c", "b"), node("x")}
	d := Depth(nodes)
	if d["a"] != 0 || d["b"] != 1 || d["c"] != 2 || d["x"] != 0 {
		t.Fatalf("层级计算错误：%v", d)
	}
}
