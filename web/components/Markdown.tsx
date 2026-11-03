import React from 'react';

// Lightweight Markdown renderer for the subset produced by the report
// generator. No dangerouslySetInnerHTML: report content can never inject HTML.

export function headingSlug(text: string): string {
  const base = text
    .replace(/^#+\s*/, '')
    .trim()
    .toLowerCase()
    .replace(/[^\w\u4e00-\u9fa5]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64);
  return `sec-${base || 'section'}`;
}

function decorate(text: string, keyPrefix: string): React.ReactNode[] {
  const out: React.ReactNode[] = [];
  const citationRe = /\[(\d+(?:\s*,\s*\d+)*)\]/g;
  let last = 0;
  let m: RegExpExecArray | null;
  let n = 0;
  while ((m = citationRe.exec(text)) !== null) {
    if (m.index > last) out.push(...bold(text.slice(last, m.index), `${keyPrefix}-b${n}`));
    const first = Number(m[1].split(',')[0].trim());
    out.push(
      <a key={`${keyPrefix}-c${n}`} className="citation-ref" href={`#cite-${first}`} title={`引用 ${m[1]}`}>
        [{m[1].replace(/\s+/g, '')}]
      </a>,
    );
    n += 1;
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push(...bold(text.slice(last), `${keyPrefix}-b${n}`));
  return out;
}

function bold(text: string, key: string): React.ReactNode[] {
  if (!text) return [];
  const parts = text.split(/\*\*([^*]+)\*\*/g);
  return parts.map((part, i) => (i % 2 === 1 ? <strong key={`${key}-${i}`}>{part}</strong> : part));
}

function inline(text: string, keyPrefix: string): React.ReactNode[] {
  const nodes: React.ReactNode[] = [];
  const linkRe = /\[([^\]]+)\]\(([^)]+)\)/g;
  let last = 0;
  let m: RegExpExecArray | null;
  let i = 0;
  while ((m = linkRe.exec(text)) !== null) {
    if (m.index > last) nodes.push(...decorate(text.slice(last, m.index), `${keyPrefix}-t${i}`));
    const [, label, href] = m;
    if (/^https?:\/\//.test(href)) {
      nodes.push(
        <a key={`${keyPrefix}-l${i}`} href={href} target="_blank" rel="noopener noreferrer">
          {label}
        </a>,
      );
    } else {
      nodes.push(label);
    }
    i += 1;
    last = m.index + m[0].length;
  }
  if (last < text.length) nodes.push(...decorate(text.slice(last), `${keyPrefix}-t${i}`));
  return nodes;
}

export function Markdown({ source, className }: { source: string; className?: string }) {
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
    if (!trimmed) { i += 1; continue; }

    if (trimmed.startsWith('### ')) {
      const text = trimmed.slice(4);
      out.push(<h3 key={`h3-${key}`} id={headingSlug(text)}>{inline(text, `h3-${key++}`)}</h3>);
      i += 1; continue;
    }
    if (trimmed.startsWith('## ')) {
      const text = trimmed.slice(3);
      out.push(<h2 key={`h2-${key}`} id={headingSlug(text)}>{inline(text, `h2-${key++}`)}</h2>);
      i += 1; continue;
    }
    if (trimmed.startsWith('# ')) {
      const text = trimmed.slice(2);
      out.push(<h1 key={`h1-${key}`} id={headingSlug(text)}>{inline(text, `h1-${key++}`)}</h1>);
      i += 1; continue;
    }
    if (trimmed.startsWith('> ')) {
      const quote: string[] = [];
      while (i < lines.length && lines[i].trim().startsWith('> ')) {
        quote.push(lines[i].trim().slice(2));
        i += 1;
      }
      out.push(<blockquote key={`q-${key++}`}>{inline(quote.join(' '), `q-${key}`)}</blockquote>);
      continue;
    }
    if (trimmed.startsWith('- ')) {
      const items: string[] = [];
      while (i < lines.length && lines[i].trim().startsWith('- ')) {
        items.push(lines[i].trim().slice(2));
        i += 1;
      }
      flushList(items, false, `ul-${key++}`);
      continue;
    }
    if (/^\d+\.\s/.test(trimmed)) {
      const items: string[] = [];
      while (i < lines.length && /^\d+\.\s/.test(lines[i].trim())) {
        items.push(lines[i].trim().replace(/^\d+\.\s/, ''));
        i += 1;
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
        i += 1;
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
    out.push(<p key={`p-${key}`}>{inline(trimmed, `p-${key++}`)}</p>);
    i += 1;
  }

  return <div className={className ?? 'md'}>{out}</div>;
}