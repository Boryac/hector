package domain

import "testing"

func TestCanTransitionAllowed(t *testing.T) {
	allowed := [][2]string{
		{TaskCreated, TaskPlanning},
		{TaskCreated, TaskCancelled},
		{TaskPlanning, TaskAwaitingApproval},
		{TaskPlanning, TaskRunning},
		{TaskAwaitingApproval, TaskRunning},
		{TaskAwaitingApproval, TaskPlanning},
		{TaskRunning, TaskSynthesizing},
		{TaskRunning, TaskPaused},
		{TaskRunning, TaskCompleted},
		{TaskRunning, TaskFailed},
		{TaskSynthesizing, TaskCompleted},
		{TaskPaused, TaskRunning},
		{TaskFailed, TaskRunning},
		{TaskFailed, TaskPlanning},
		{TaskFailed, TaskCancelled},
	}
	for _, c := range allowed {
		if !CanTransition(c[0], c[1]) {
			t.Fatalf("白名单应允许 %s → %s", c[0], c[1])
		}
	}
}

func TestCanTransitionRejected(t *testing.T) {
	rejected := [][2]string{
		{TaskCreated, TaskRunning},         // 不能跳过规划
		{TaskCreated, TaskCompleted},       // 不能跳过执行
		{TaskCompleted, TaskRunning},       // 终态不可复活
		{TaskCancelled, TaskRunning},       // 终态不可复活
		{TaskPaused, TaskSynthesizing},     // 暂停不能直接收尾
		{TaskAwaitingApproval, TaskPaused}, // 未开始不能暂停
	}
	for _, c := range rejected {
		if CanTransition(c[0], c[1]) {
			t.Fatalf("白名单不应允许 %s → %s", c[0], c[1])
		}
	}
}

func TestCanTransitionIdempotent(t *testing.T) {
	for _, s := range TaskStatuses() {
		if !CanTransition(s, s) {
			t.Fatalf("同状态跃迁应幂等通过：%s", s)
		}
	}
}

func TestIsTerminal(t *testing.T) {
	if !IsTerminal(TaskCompleted) || !IsTerminal(TaskCancelled) {
		t.Fatal("completed / cancelled 应为终态")
	}
	for _, s := range []string{TaskCreated, TaskPlanning, TaskAwaitingApproval, TaskRunning, TaskPaused, TaskSynthesizing, TaskFailed} {
		if IsTerminal(s) {
			t.Fatalf("%s 不应是终态", s)
		}
	}
}

func TestEventForTransition(t *testing.T) {
	cases := map[string]string{
		TaskPlanning:     EvtResearchPlanning,
		TaskRunning:      EvtResearchStarted,
		TaskPaused:       EvtResearchPaused,
		TaskSynthesizing: EvtResearchSynthesizing,
		TaskCompleted:    EvtResearchCompleted,
		TaskFailed:       EvtResearchFailed,
		TaskCancelled:    EvtResearchCancelled,
	}
	for to, want := range cases {
		if got := EventForTransition(to); got != want {
			t.Fatalf("EventForTransition(%s) = %q，期望 %q", to, got, want)
		}
	}
}

func TestErrInvalidTransitionMessage(t *testing.T) {
	err := &ErrInvalidTransition{TaskID: "t1", From: TaskCompleted, To: TaskRunning}
	msg := err.Error()
	if msg == "" {
		t.Fatal("错误信息不能为空")
	}
	for _, want := range []string{"t1", TaskCompleted, TaskRunning} {
		contains := false
		for i := 0; i+len(want) <= len(msg); i++ {
			if msg[i:i+len(want)] == want {
				contains = true
				break
			}
		}
		if !contains {
			t.Fatalf("错误信息 %q 缺少 %q", msg, want)
		}
	}
}

func TestTaskStatusesCoverTable(t *testing.T) {
	statuses := TaskStatuses()
	if len(statuses) != len(taskTransitions) {
		t.Fatalf("TaskStatuses 数量 %d 与状态机表 %d 不一致", len(statuses), len(taskTransitions))
	}
	for _, s := range statuses {
		if _, ok := taskTransitions[s]; !ok {
			t.Fatalf("状态 %s 在状态机表中缺失", s)
		}
	}
}
