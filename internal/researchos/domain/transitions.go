package domain

import "fmt"

// ErrInvalidTransition 表示任务状态跃迁不在允许的白名单内。
//
// 状态机是唯一入口约束：任何变更必须经 Store.TransitionTask，禁止直接 UPDATE status。
type ErrInvalidTransition struct {
	TaskID string
	From   string
	To     string
}

func (e *ErrInvalidTransition) Error() string {
	return fmt.Sprintf("任务 %s 不允许从 %q 跃迁到 %q", e.TaskID, e.From, e.To)
}

// taskTransitions 是任务生命周期白名单。
//
// 设计口径（与 DATA_MODEL.md 第 3 节一致）：
//   - created 只能进入 planning，或直接终止；
//   - planning 可因需人工确认进入 awaiting_approval，也可直接 running（自动批准模式）；
//   - running 是唯一可以批量推进节点的状态；synthesizing 用于收尾阶段；
//   - paused 只能来自 running / synthesizing，恢复后回到 running；
//   - completed / cancelled 为终态；failed 仅允许人工取消收尾。
var taskTransitions = map[string][]string{
	TaskCreated: {
		TaskPlanning, TaskCancelled, TaskFailed,
	},
	TaskPlanning: {
		TaskAwaitingApproval, TaskRunning, TaskCancelled, TaskFailed,
	},
	TaskAwaitingApproval: {
		TaskRunning, TaskPlanning, TaskCancelled, TaskFailed,
	},
	TaskRunning: {
		TaskSynthesizing, TaskPaused, TaskCompleted, TaskFailed, TaskCancelled,
	},
	TaskSynthesizing: {
		TaskCompleted, TaskPaused, TaskFailed, TaskCancelled,
	},
	TaskPaused: {
		TaskRunning, TaskCancelled, TaskFailed,
	},
	TaskCompleted: {},
	TaskFailed:    {TaskCancelled, TaskRunning, TaskPlanning},
	TaskCancelled: {},
}

// CanTransition 判断 from → to 是否合法。相同状态视为幂等成功（不写事件）。
func CanTransition(from, to string) bool {
	if from == to {
		return true
	}
	allowed, ok := taskTransitions[from]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == to {
			return true
		}
	}
	return false
}

// IsTerminal 判断任务是否已进入终态。
func IsTerminal(status string) bool {
	return status == TaskCompleted || status == TaskCancelled
}

// TaskStatuses 返回全部合法状态，供 API 校验 query 参数。
func TaskStatuses() []string {
	return []string{
		TaskCreated, TaskPlanning, TaskAwaitingApproval, TaskRunning,
		TaskPaused, TaskSynthesizing, TaskCompleted, TaskFailed, TaskCancelled,
	}
}

// EventForTransition 返回跃迁对应的默认事件类型。
func EventForTransition(to string) string {
	switch to {
	case TaskPlanning:
		return EvtResearchPlanning
	case TaskRunning:
		return EvtResearchStarted
	case TaskPaused:
		return EvtResearchPaused
	case TaskCompleted:
		return EvtResearchCompleted
	case TaskFailed:
		return EvtResearchFailed
	case TaskSynthesizing:
		return EvtResearchSynthesizing
	case TaskCancelled:
		return EvtResearchCancelled
	default:
		return ""
	}
}
