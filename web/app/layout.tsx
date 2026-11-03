import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Hector ResearchOS',
  description: '自主研究工作台：规划 → 检索 → 证据 → 报告',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="zh-CN">
      <body>{children}</body>
    </html>
  );
}
