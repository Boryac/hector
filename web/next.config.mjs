/** @type {import('next').NextConfig} */
const nextConfig = {
  // 静态导出：产物可直接由 Go 服务的 RESEARCHOS_WEB_DIR 托管，也可用 nginx / Docker 部署。
  output: 'export',
  trailingSlash: true,
  distDir: '.build',
  images: { unoptimized: true },
  reactStrictMode: true,
};

export default nextConfig;
