-- 设置中心：让运行配置可以被 Web 界面管理，而不只是环境变量。
--
-- 设计要点（见 DECISIONS.md 第 29-32 条）：
--   1. app_settings 仍是唯一存储，新增 section / encrypted / updated_by 三列；
--   2. 敏感值以密文写入 value（形如 "enc:v1:<base64>"），encrypted 标记便于审计，
--      但**不**信任该标记做解密判断——解密看的是值前缀；
--   3. mcp_servers 增加 tools 缓存与 last_error，供 MCP Hub 展示发现结果。

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS section    TEXT    NOT NULL DEFAULT 'general',
    ADD COLUMN IF NOT EXISTS encrypted  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS updated_by TEXT    NOT NULL DEFAULT 'local-user';

CREATE INDEX IF NOT EXISTS idx_app_settings_section ON app_settings (section);

ALTER TABLE mcp_servers
    ADD COLUMN IF NOT EXISTS tools      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS last_error TEXT        NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
