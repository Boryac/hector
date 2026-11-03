import React from 'react';

// 轻量 Markdown 渲染（仅支持后端报告生成器实际使用的子集）。
// 不使用 dangerouslySetInnerHTML，避免任何注入风险。

function inline(text: string, keyPrefix: string): React.ReactNode[] {
  const nodes: React.ReactNode[] = [];
  const linkRe = /\[([^\]]+)\]\(([^)]+)\)/g;
  let last = 0;
  let m: RegExpExecArray | null;
  let i = 0;
  while ((m = linkRe.exec(text)) !== null) {
    if (m.index > last) nodes.push(text.slice(last, m.index));
    const [, label, href] = m;
    if (/^https?:\/\//.test(href)) {
      nodes.push(
        <a key={`${keyPrefix}-l${i++}`} href={href} target="_blank" rel="noopener noreferrer">
          {label}
        </a>,
      );
    } else {
      nodes.push(label);
    }
    last = m.index + m[0].length;
  }
  if (last < text.length) nodes.push(text.slice(last));

  // 处理加粗
  const result: React.ReactNode[] = [];
  nodes.forEach((node, idx) => {
    if (typeof node !== 'string') {
      result.push(node);
      return;
    }
    const parts = node.split(/\*\*([^*]+)\*\*/g);
    parts.forEach((p, j) => {
      result.push(j % 2 === 1 ? <strong key={`${keyPrefix}-b${idx}-${j}`}>{p}</strong> : p);
    });
  });
  return result;
}

export function Markdown({ source }: { source: string }) {
  const lines = source.split('\n');
  const out: React.ReactNode[] = [];
  let i = 0;
  let key = 0;

  const flushList = (items: string[], ordered: boolean, k: string) => {
    if (items.length === 0) return;
    const children = items.map((it, idx) => <li key={`${k}-${idx}`}>{inline(it, `${k}-${idx}`)}</li>);
    out.push(ordered ? <ol key={k}>{children}</ol> : <ul key={k}>{children}</ul>);
  };

  while (i < lines.length) {
    const line = lines[i].trimEnd();
    const trimmed = line.trim();
    if (!trimmed) { i++; continue; }

    if (trimmed.startsWith('### ')) {
      out.push(<h3 key={`h3-${key++}`}>{inline(trimmed.slice(4), `h3-${key}`)}</h3>);
      i++; continue;
    }
    if (trimmed.startsWith('## ')) {
      out.push(<h2 key={`h2-${key++}`}>{inline(trimmed.slice(3), `h2-${key}`)}</h2>);
      i++; continue;
    }
    if (trimmed.startsWith('# ')) {
      out.push(<h1 key={`h1-${key++}`}>{inline(trimmed.slice(2), `h1-${key}`)}</h1>);
      i++; continue;
    }
    if (trimmed.startsWith('- ')) {
      const items: string[] = [];
      while (i < lines.length && lines[i].trim().startsWith('- ')) {
        items.push(lines[i].trim().slice(2));
        i++;
      }
      flushList(items, false, `ul-${key++}`);
      continue;
    }
    if (/^\d+\.\s/.test(trimmed)) {
      const items: string[] = [];
      while (i < lines.length && /^\d+\.\s/.test(lines[i].trim())) {
        items.push(lines[i].trim().replace(/^\d+\.\s/, ''));
        i++;
      }
      flushList(items, true, `ol-${key++}`);
      continue;
    }
    if (trimmed.startsWith('|') && i + 1 < lines.length && /^\|[\s:|-]+\|$/.test(lines[i + 1].trim())) {
      const header = trimmed.replace(/^\||\|$/g, '').split('|').map((s) => s.trim());
      i += 2;
      const rows: string[][] = [];
      while (i < lines.length && lines[i].trim().startsWith('|')) {
        rows.push(lines[i].trim().replace(/^\||\|$/g, '').split('|').map((s) => s.trim()));
        i++;
      }
      out.push(
        <table key={`tbl-${key++}`}>
          <thead>
            <tr>{header.map((h, idx) => <th key={idx}>{inline(h, `th-${idx}`)}</th>)}</tr>
          </thead>
          <tbody>
            {rows.map((r, ri) => (
              <tr key={ri}>{r.map((c, ci) => <td key={ci}>{inline(c, `td-${ri}-${ci}`)}</td>)}</tr>
            ))}
          </tbody>
        </table>,
      );
      continue;
    }
    out.push(<p key={`p-${key++}`}>{inline(trimmed, `p-${key}`)}</p>);
    i++;
  }
  return <div className="report">{out}</div>;
}
