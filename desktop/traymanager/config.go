package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// config.go — 应用级配置与用户数据目录管理。
//
// 目录模型（发行目录与用户数据严格分离）：
//   应用级配置: %LOCALAPPDATA%\Hector ResearchOS\config.json（含 DPAPI 加密凭据）
//   用户数据:   用户首次启动时选择，默认 %LOCALAPPDATA%\Hector ResearchOS\data
//               其下分: db\(PostgreSQL 逻辑库以外的运行数据) logs\ run\(pid) tmp\
//               注意: PostgreSQL 物理数据目录由外置 PostgreSQL 自行管理，本应用绝不触碰。

type Config struct {
	Version     int    `json:"version"` // 配置结构版本，升级用
	Initialized bool   `json:"initialized"`
	DataDir     string `json:"dataDir"`
	Addr        string `json:"addr"` // 后端监听地址，默认 127.0.0.1:8080
	Autostart   bool   `json:"autostart"`

	// 敏感字段：以 DPAPI 加密后 base64 存储
	PgDSN     string `json:"pgDsnEnc,omitempty"`     // RESEARCHOS_DATABASE_URL
	APIToken  string `json:"apiTokenEnc,omitempty"`  // RESEARCHOS_API_TOKEN
	SecretKey string `json:"secretKeyEnc,omitempty"` // RESEARCHOS_SECRET_KEY（API Key 加密主钥）
}

// appConfigDir 返回应用级配置目录。
func appConfigDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return "", errors.New("LOCALAPPDATA 未设置")
	}
	return filepath.Join(base, "Hector ResearchOS"), nil
}

func appConfigPath() (string, error) {
	dir, err := appConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// LoadConfig 读取配置；文件不存在返回 (nil, nil)。
func LoadConfig() (*Config, error) {
	p, err := appConfigPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("config.json 损坏: %w", err)
	}
	return &c, nil
}

// SaveConfig 原子写入配置（同目录临时文件 + rename）。
func SaveConfig(c *Config) error {
	p, err := appConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ProtectConfig 将敏感明文字段 DPAPI 加密后写回结构体。
func ProtectConfig(c *Config) error {
	enc, err := encryptField(c.PgDSN)
	if err != nil {
		return err
	}
	c.PgDSN = enc
	if c.APIToken, err = encryptField(c.APIToken); err != nil {
		return err
	}
	if c.SecretKey, err = encryptField(c.SecretKey); err != nil {
		return err
	}
	return nil
}

// UnprotectConfig 还原敏感字段为明文（仅本进程内存使用）。
func UnprotectConfig(c *Config) error {
	dec, err := decryptField(c.PgDSN)
	if err != nil {
		return err
	}
	c.PgDSN = dec
	if c.APIToken, err = decryptField(c.APIToken); err != nil {
		return err
	}
	if c.SecretKey, err = decryptField(c.SecretKey); err != nil {
		return err
	}
	return nil
}

func encryptField(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	blob, err := dpapiProtect([]byte(plain))
	if err != nil {
		return "", err
	}
	return "dpapi:" + base64.StdEncoding.EncodeToString(blob), nil
}

func decryptField(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, "dpapi:") {
		return "", errors.New("敏感字段缺少 dpapi 前缀（疑似被手改）")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "dpapi:"))
	if err != nil {
		return "", err
	}
	plain, err := dpapiUnprotect(raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DefaultDataDir 首次启动的推荐数据目录。
func DefaultDataDir() (string, error) {
	dir, err := appConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data"), nil
}

// ValidateDataDir 首次启动检查：存在/可写/空间/冲突数据。
func ValidateDataDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("数据目录不能为空")
	}
	if fi, err := os.Stat(dir); err == nil {
		if !fi.IsDir() {
			return fmt.Errorf("%s 已存在且不是目录", dir)
		}
	} else if os.IsNotExist(err) {
		// 目录不存在属正常，稍后创建
	} else {
		return err
	}
	probe := filepath.Join(dir, ".write-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("无法创建目录: %w", err)
	}
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("目录不可写: %w", err)
	}
	_ = os.Remove(probe)
	return nil
}

// RandomToken 生成 32 字节十六进制随机串。
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnsureSubdirs 创建数据目录下的固定子结构。
func EnsureSubdirs(dataDir string) error {
	for _, sub := range []string{"logs", "run", "tmp"} {
		if err := os.MkdirAll(filepath.Join(dataDir, sub), 0o755); err != nil {
			return err
		}
	}
	return nil
}
