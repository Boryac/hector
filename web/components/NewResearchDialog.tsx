'use client';

import React, { useState } from 'react';
import { useRouter } from 'next/navigation';
import { apiSend } from '@/lib/api';
import type { Task } from '@/lib/types';
import { Button, Field, Modal, Segmented, Switch } from './ui';
import { useToast } from './Toast';
import { Icon } from './Icon';

const DEPTHS = [
  { value: 'quick' as const, label: '快速' },
  { value: 'standard' as const, label: '标准' },
  { value: 'deep' as const, label: '深度' },
];

export function NewResearchDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const router = useRouter();
  const { push } = useToast();
  const [goal, setGoal] = useState('');
  const [title, setTitle] = useState('');
  const [depth, setDepth] = useState<'quick' | 'standard' | 'deep'>('standard');
  const [verify, setVerify] = useState(true);
  const [maxAgents, setMaxAgents] = useState(4);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const close = () => {
    if (submitting) return;
    setError(null);
    onClose();
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = goal.trim();
    if (trimmed.length < 4) {
      setError('研究目标至少需要 4 个字符。');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const res = await apiSend<{ task: Task }>('POST', '/api/v1/research', {
        goal: trimmed,
        title: title.trim() || undefined,
        depth,
        enable_verification: verify,
        max_agents: maxAgents,
        plan_now: true,
      });
      push('研究任务已创建，正在生成规划。', 'ok');
      setGoal('');
      setTitle('');
      onClose();
      router.push(`/task/?id=${res.task.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={close}
      title="新建研究"
      description="描述需要回答的问题。系统将先生成规划，经你批准后开始执行。"
      width={600}
      footer={
        <div className="modal-actions">
          <Button variant="ghost" onClick={close} disabled={submitting}>取消</Button>
          <Button variant="primary" onClick={submit} loading={submitting} icon={<Icon name="arrow-right" size={15} />}>
            创建并规划
          </Button>
        </div>
      }
    >
      <form className="form-stack" onSubmit={submit}>
        <Field
          label="研究目标"
          htmlFor="research-goal"
          hint="写清楚对象、范围与期望产出，规划质量会明显更好。"
        >
          <textarea
            id="research-goal"
            className="input textarea"
            rows={5}
            value={goal}
            onChange={(e) => setGoal(e.target.value)}
            placeholder="例如：对比 pgvector 与 Qdrant 在千万级向量检索下的召回率与写入吞吐，并给出选型建议。"
            autoFocus
          />
        </Field>

        <Field label="标题（可选）" htmlFor="research-title" hint="留空时使用研究目标作为标题。">
          <input
            id="research-title"
            className="input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="本轮研究的短标题"
          />
        </Field>

        <div className="form-grid">
          <Field label="研究深度" hint="深度决定检索与分析节点的数量。">
            <Segmented value={depth} options={DEPTHS} onChange={setDepth} ariaLabel="研究深度" />
          </Field>
          <Field label="并发 Agent" hint="本次任务允许同时执行的 Agent 数量，1–8。">
            <input
              className="input"
              type="number"
              min={1}
              max={8}
              value={maxAgents}
              onChange={(e) => setMaxAgents(Math.max(1, Math.min(8, Number(e.target.value) || 1)))}
            />
          </Field>
        </div>

        <Switch
          checked={verify}
          onChange={setVerify}
          label="启用交叉验证"
          description="对结论进行多来源核验；关闭后报告会明确标注结论强度有限。"
        />

        {error && <div className="form-error" role="alert">{error}</div>}
        <button type="submit" hidden aria-hidden="true" tabIndex={-1} />
      </form>
    </Modal>
  );
}