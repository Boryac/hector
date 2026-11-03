package exec

import "errors"

// PermanentError 表示「再试一次结果也一样」的失败。
//
// 典型场景：节点拿不到任何结论（上游检索全空）、节点类型未知、节点缺少必要输入。
// 这类错误与网络抖动不同，重试只会把同样的计算再跑 N 遍——实测中
// `synthesize` 因「没有结论可生成报告」被重试到第 3 次才收手，白等两轮退避。
//
// 用法：在会确定失败的分支 `return nil, 0, Permanent(err)`；
// Worker 用 IsPermanent 判断后直接失败，不再排重试。
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }

func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent 把错误标记为不可重试。err 为 nil 时返回 nil。
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Err: err}
}

// IsPermanent 判断错误是否不可重试（含被包装的情况）。
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}
