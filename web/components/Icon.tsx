import React from 'react';

export type IconName =
  | 'overview' | 'tasks' | 'sources' | 'evidence' | 'reports' | 'settings'
  | 'search' | 'plus' | 'refresh' | 'chevron-down' | 'chevron-right' | 'arrow-left'
  | 'arrow-right' | 'x' | 'play' | 'pause' | 'stop' | 'retry' | 'external'
  | 'check' | 'alert' | 'clock' | 'database' | 'cpu' | 'key' | 'download'
  | 'filter' | 'activity' | 'layers' | 'menu' | 'link' | 'copy' | 'info'
  | 'dot' | 'layout';

const PATHS: Record<IconName, React.ReactNode> = {
  overview: (
    <>
      <rect x="3" y="3" width="7" height="7" rx="1.4" />
      <rect x="14" y="3" width="7" height="7" rx="1.4" />
      <rect x="3" y="14" width="7" height="7" rx="1.4" />
      <rect x="14" y="14" width="7" height="7" rx="1.4" />
    </>
  ),
  tasks: (
    <>
      <path d="M9 6h12M9 12h12M9 18h12" />
      <circle cx="4.4" cy="6" r="1.1" />
      <circle cx="4.4" cy="12" r="1.1" />
      <circle cx="4.4" cy="18" r="1.1" />
    </>
  ),
  sources: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3c2.6 2.5 4 5.6 4 9s-1.4 6.5-4 9c-2.6-2.5-4-5.6-4-9s1.4-6.5 4-9Z" />
    </>
  ),
  evidence: (
    <>
      <path d="M4 5.5h16v10.5H9l-5 4.2V5.5Z" />
      <path d="M9.5 9.2h5M9.5 12.2h3.2" />
    </>
  ),
  reports: (
    <>
      <path d="M6 3.5h7.5L18 8v12.5H6V3.5Z" />
      <path d="M13.5 3.5V8H18M9 12h6M9 15.5h6M9 8.5h2.5" />
    </>
  ),
  settings: (
    <>
      <path d="M4 7h9M17 7h3M4 17h3M11 17h9" />
      <circle cx="15" cy="7" r="2.2" />
      <circle cx="9" cy="17" r="2.2" />
    </>
  ),
  search: (
    <>
      <circle cx="10.8" cy="10.8" r="6.3" />
      <path d="m15.5 15.5 4.2 4.2" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  refresh: (
    <>
      <path d="M20 11a8 8 0 0 0-14.6-4.6L3 9" />
      <path d="M3 4v5h5M4 13a8 8 0 0 0 14.6 4.6L21 15" />
      <path d="M21 20v-5h-5" />
    </>
  ),
  'chevron-down': <path d="m6 9 6 6 6-6" />,
  'chevron-right': <path d="m9 6 6 6-6 6" />,
  'arrow-left': <path d="M19 12H5m6-6-6 6 6 6" />,
  'arrow-right': <path d="M5 12h14m-6-6 6 6-6 6" />,
  x: <path d="m6 6 12 12M18 6 6 18" />,
  play: <path d="M8 5.5v13l10-6.5-10-6.5Z" />,
  pause: <path d="M9 5.5v13M15 5.5v13" />,
  stop: <rect x="6" y="6" width="12" height="12" rx="1.5" />,
  retry: (
    <>
      <path d="M3 12a9 9 0 1 0 3-6.7" />
      <path d="M3 4v5h5" />
    </>
  ),
  external: (
    <>
      <path d="M14 4h6v6M20 4l-8.5 8.5" />
      <path d="M19 14v5.5H4.5V5H10" />
    </>
  ),
  check: <path d="m5 12.5 4.2 4.2L19 7" />,
  alert: (
    <>
      <path d="M12 4 2.8 20h18.4L12 4Z" />
      <path d="M12 9.5v4.5M12 17.2v.1" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="8.6" />
      <path d="M12 7v5.3l3.5 2" />
    </>
  ),
  database: (
    <>
      <ellipse cx="12" cy="5.6" rx="7.4" ry="2.8" />
      <path d="M4.6 5.6v12.8c0 1.6 3.3 2.8 7.4 2.8s7.4-1.2 7.4-2.8V5.6" />
      <path d="M4.6 12c0 1.6 3.3 2.8 7.4 2.8s7.4-1.2 7.4-2.8" />
    </>
  ),
  cpu: (
    <>
      <rect x="6.5" y="6.5" width="11" height="11" rx="1.6" />
      <rect x="10" y="10" width="4" height="4" rx="0.8" />
      <path d="M9 3v3.5M15 3v3.5M9 17.5V21M15 17.5V21M3 9h3.5M3 15h3.5M17.5 9H21M17.5 15H21" />
    </>
  ),
  key: (
    <>
      <circle cx="8" cy="15" r="4" />
      <path d="m11 12 8-8M16.5 6.5 19 9M14 9l2 2" />
    </>
  ),
  download: (
    <>
      <path d="M12 3.5v11M7.5 10l4.5 4.5 4.5-4.5" />
      <path d="M4.5 17.5v3h15v-3" />
    </>
  ),
  filter: <path d="M3.5 5.5h17l-6.6 7.6v6l-3.8 2v-8L3.5 5.5Z" />,
  activity: <path d="M3 12h4l2.6-6.5 4.8 13 2.6-6.5h4" />,
  layers: (
    <>
      <path d="m12 3.5 8.5 4.6L12 12.7 3.5 8.1 12 3.5Z" />
      <path d="m3.5 12.2 8.5 4.6 8.5-4.6M3.5 16.3l8.5 4.6 8.5-4.6" />
    </>
  ),
  menu: <path d="M4 7h16M4 12h16M4 17h16" />,
  link: (
    <>
      <path d="M10 14a4.2 4.2 0 0 0 6 0l2.4-2.4a4.2 4.2 0 0 0-6-6L11 7" />
      <path d="M14 10a4.2 4.2 0 0 0-6 0l-2.4 2.4a4.2 4.2 0 0 0 6 6L13 17" />
    </>
  ),
  copy: (
    <>
      <rect x="9" y="9" width="11" height="11" rx="1.6" />
      <path d="M15 9V5.5A1.5 1.5 0 0 0 13.5 4h-8A1.5 1.5 0 0 0 4 5.5v8A1.5 1.5 0 0 0 5.5 15H9" />
    </>
  ),
  info: (
    <>
      <circle cx="12" cy="12" r="8.6" />
      <path d="M12 10.8v5M12 7.6v.1" />
    </>
  ),
  dot: <circle cx="12" cy="12" r="3.2" fill="currentColor" stroke="none" />,
  layout: (
    <>
      <rect x="3.5" y="4" width="17" height="16" rx="2" />
      <path d="M3.5 9h17M9.5 9v11" />
    </>
  ),
};

export function Icon({
  name,
  size = 16,
  className,
  strokeWidth = 1.6,
}: {
  name: IconName;
  size?: number;
  className?: string;
  strokeWidth?: number;
}) {
  return (
    <svg
      className={className}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {PATHS[name]}
    </svg>
  );
}

// 品牌标记：两个立柱与横梁构成的抽象 H，不依赖字体或图片资源。
export function BrandMark({ size = 30 }: { size?: number }) {
  return (
    <span className="brand-mark" style={{ width: size, height: size }}>
      <svg width={size * 0.56} height={size * 0.56} viewBox="0 0 18 18" fill="none" aria-hidden="true">
        <path d="M3.4 3v12M14.6 3v12M3.4 9h11.2" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" />
      </svg>
    </span>
  );
}