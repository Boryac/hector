package exec

import (
	"errors"
	"fmt"
	"testing"
)

func TestPermanentClassification(t *testing.T) {
	if IsPermanent(nil) {
		t.Fatal("nil 不应判为永久失败")
	}
	if IsPermanent(errors.New("临时错误")) {
		t.Fatal("普通错误不应判为永久失败")
	}
	if !IsPermanent(Permanent(errors.New("确定性错误"))) {
		t.Fatal("Permanent 标记的错误应被识别")
	}
	// 被 %w 包装后仍要能识别：调用链上常会再包一层上下文。
	wrapped := fmt.Errorf("节点失败: %w", Permanent(errors.New("没有结论")))
	if !IsPermanent(wrapped) {
		t.Fatal("包装后的永久错误应被识别")
	}
	if got := wrapped.Error(); got != "节点失败: 没有结论" {
		t.Fatalf("错误文案不应被污染：%q", got)
	}
	// 底层原因可用 errors.Is 命中，便于调用方细判。
	sentinel := errors.New("sentinel")
	if !errors.Is(Permanent(sentinel), sentinel) {
		t.Fatal("Permanent 应保留底层错误链")
	}
	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) 应返回 nil")
	}
}
