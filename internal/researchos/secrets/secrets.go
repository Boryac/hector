// Package secrets 负责敏感配置（API Key 等）的静态加密与掩码。
//
// 威胁模型与取舍（见 DECISIONS.md 第 30 条）：
//   - 数据库文件、备份、`pg_dump` 产物都可能被拷走，明文存 Key 等于把凭据一并送人；
//   - 因此敏感值一律用 AES-256-GCM 加密后入库，密文形如 `enc:v1:<base64(nonce|ciphertext)>`；
//   - 主密钥优先级：环境变量 `RESEARCHOS_SECRET_KEY` > 数据目录下的 `secret.key`；
//   - 两者都没有时**生成**一个并写盘（0600），而不是拒绝启动——拒绝启动会让
//     「普通用户不改 .env 也能用」这个目标直接落空。
//
// 明确的边界：本方案防的是「数据库泄露」，不是「主机被攻陷」。攻击者拿到
// 进程可读的文件（含 secret.key）仍可解密，这一点不假装能防。
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// Prefix 标记密文，同时充当版本号，便于将来换算法。
	Prefix = "enc:v1:"
	// KeySize 是 AES-256 要求的密钥长度。
	KeySize = 32
	// EnvKey 是主密钥的环境变量名。
	EnvKey = "RESEARCHOS_SECRET_KEY"
	// KeyFileName 是数据目录下的主密钥文件名。
	KeyFileName = "secret.key"
)

var (
	ErrNoKey      = errors.New("没有可用的主密钥")
	ErrBadKey     = errors.New("主密钥长度不正确（需要 32 字节）")
	ErrBadPayload = errors.New("密文格式不正确")
	ErrDecrypt    = errors.New("解密失败：主密钥不匹配或密文已被篡改")
)

// Box 用一把主密钥做加解密。
type Box struct {
	aead cipher.AEAD
}

// NewBox 用 32 字节主密钥构造 Box。
func NewBox(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, ErrBadKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// ParseKey 解析文本形式的主密钥：优先按 base64（标准/URL 安全），再按 hex。
func ParseKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrNoKey
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding,
	} {
		if raw, err := enc.DecodeString(s); err == nil && len(raw) == KeySize {
			return raw, nil
		}
	}
	if raw, err := hex.DecodeString(s); err == nil && len(raw) == KeySize {
		return raw, nil
	}
	return nil, ErrBadKey
}

// IsEncrypted 判断一个值是否为本包产出的密文。
func IsEncrypted(v string) bool { return strings.HasPrefix(v, Prefix) }

// Seal 加密明文，返回带前缀的密文。
func (b *Box) Seal(plain string) (string, error) {
	if b == nil || b.aead == nil {
		return "", ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := b.aead.Seal(nil, nonce, []byte(plain), nil)
	return Prefix + base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

// Open 解密。为了兼容历史明文数据，非密文原样返回——不报错，
// 但调用方可以用 IsEncrypted 判断这条值到底有没有被保护过。
func (b *Box) Open(value string) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}
	if b == nil || b.aead == nil {
		return "", ErrNoKey
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, Prefix))
	if err != nil {
		return "", ErrBadPayload
	}
	ns := b.aead.NonceSize()
	if len(raw) < ns+1 {
		return "", ErrBadPayload
	}
	plain, err := b.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plain), nil
}

// LoadOrCreateKey 从 keyFile 读取主密钥；不存在则生成并落盘（0600）。
// 返回 created 表示本次是否新生成。
func LoadOrCreateKey(keyFile string) (key []byte, created bool, err error) {
	if raw, err := os.ReadFile(keyFile); err == nil {
		k, perr := ParseKey(string(raw))
		if perr != nil {
			return nil, false, fmt.Errorf("读取 %s 失败: %w", keyFile, perr)
		}
		return k, false, nil
	} else if !os.IsNotExist(err) {
		return nil, false, err
	}

	key = make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(keyFile), 0o700); err != nil {
		return nil, false, err
	}
	// 先写临时文件再改名，避免并发启动时读到半截密钥。
	tmp := keyFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		return nil, false, err
	}
	if err := os.Rename(tmp, keyFile); err != nil {
		return nil, false, err
	}
	return key, true, nil
}

// FromEnvOrFile 按优先级取主密钥：环境变量 > 密钥文件。
// 返回 source 用于在系统设置页如实展示「密钥来自哪里」。
func FromEnvOrFile(envValue, keyFile string) (*Box, string, error) {
	if strings.TrimSpace(envValue) != "" {
		key, err := ParseKey(envValue)
		if err != nil {
			return nil, "env", err
		}
		box, err := NewBox(key)
		return box, "env:" + EnvKey, err
	}
	key, created, err := LoadOrCreateKey(keyFile)
	if err != nil {
		return nil, "file", err
	}
	box, err := NewBox(key)
	if err != nil {
		return nil, "file", err
	}
	if created {
		return box, "file:" + keyFile + "（本次新生成）", nil
	}
	return box, "file:" + keyFile, nil
}

// Mask 返回可安全展示的掩码。前后各留 4 位便于用户确认「是不是这把 Key」，
// 长度不足 12 时只显示固定串——短串保留前后缀等于直接暴露。
func Mask(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	r := []rune(v)
	if len(r) < 12 {
		return "****"
	}
	return string(r[:4]) + "****" + string(r[len(r)-4:])
}
