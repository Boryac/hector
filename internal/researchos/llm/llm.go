// Package llm 提供 OpenAI 兼容的对话补全客户端（OpenAI / DeepSeek / Ollama 等）。
//
// 说明：ResearchOS 需要的是「确定性 JSON 输出 + Token 计量」，
// 与上游 pkg/model 面向 ADK 多轮工具调用的接口形状不同，故单独实现薄客户端；
// 该取舍记录在 DECISIONS.md（第 17 条）。
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Usage 是 Token 计量。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Client 是 LLM 客户端接口。
type Client interface {
	// Available 表示是否具备真实调用条件（有凭据）。
	Available() bool
	// Model 返回模型名（用于报告与 UI 标注）。
	Model() string
	// Complete 返回纯文本补全。
	Complete(ctx context.Context, system, user string) (string, Usage, error)
	// CompleteJSON 要求模型返回 JSON 对象（失败时返回错误，由调用方降级）。
	CompleteJSON(ctx context.Context, system, user string) (json.RawMessage, Usage, error)
}

// Config 是客户端配置。
type Config struct {
	Provider string
	APIKey   string
	Model    string
	BaseURL  string
	Timeout  time.Duration
}

// New 按配置构造客户端；凭据缺失时返回 Unavailable 客户端（Available()==false），
// 而不是 nil，避免调用方到处判空。
func New(cfg Config) Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if cfg.BaseURL == "" {
		switch strings.ToLower(cfg.Provider) {
		case "deepseek":
			cfg.BaseURL = "https://api.deepseek.com/v1"
		case "ollama":
			cfg.BaseURL = "http://127.0.0.1:11434/v1"
		default:
			cfg.BaseURL = "https://api.openai.com/v1"
		}
	}
	if cfg.Model == "" {
		switch strings.ToLower(cfg.Provider) {
		case "deepseek":
			cfg.Model = "deepseek-chat"
		case "ollama":
			cfg.Model = "qwen2.5:7b"
		default:
			cfg.Model = "gpt-4o-mini"
		}
	}
	c := &openAICompat{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
	if cfg.APIKey == "" && strings.ToLower(cfg.Provider) != "ollama" {
		c.disabled = "未配置 LLM API Key"
	}
	return c
}

type openAICompat struct {
	cfg      Config
	http     *http.Client
	disabled string
}

func (c *openAICompat) Available() bool { return c.disabled == "" }

func (c *openAICompat) Model() string { return c.cfg.Model }

func (c *openAICompat) Complete(ctx context.Context, system, user string) (string, Usage, error) {
	return c.do(ctx, system, user, false)
}

func (c *openAICompat) CompleteJSON(ctx context.Context, system, user string) (json.RawMessage, Usage, error) {
	text, usage, err := c.do(ctx, system, user, true)
	if err != nil {
		return nil, usage, err
	}
	raw, err := extractJSONObject(text)
	if err != nil {
		return nil, usage, err
	}
	return raw, usage, nil
}

func (c *openAICompat) do(ctx context.Context, system, user string, jsonMode bool) (string, Usage, error) {
	if !c.Available() {
		return "", Usage{}, errors.New(c.disabled)
	}
	body := map[string]any{
		"model": c.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
	}
	if jsonMode {
		body["response_format"] = map[string]string{"type": "json_object"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", Usage{}, err
	}
	endpoint := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", Usage{}, fmt.Errorf("LLM 请求失败: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		// 错误信息里可能含 key 片段，截断后返回，不落日志明文。
		return "", Usage{}, fmt.Errorf("LLM 返回 %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", Usage{}, fmt.Errorf("LLM 响应解析失败: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", out.Usage, errors.New("LLM 返回空 choices")
	}
	return out.Choices[0].Message.Content, out.Usage, nil
}

// extractJSONObject 从模型输出中提取第一个完整 JSON 对象（容忍 ```json 包裹与前后废话）。
func extractJSONObject(s string) (json.RawMessage, error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		rest := s[i+3:]
		if j := strings.Index(rest, "```"); j >= 0 {
			rest = rest[:j]
		}
		rest = strings.TrimPrefix(rest, "json")
		s = strings.TrimSpace(rest)
	}
	start := strings.Index(s, "{")
	if start < 0 {
		return nil, errors.New("LLM 输出不含 JSON 对象")
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if ch == '\\' {
				esc = true
				continue
			}
			if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				candidate := []byte(s[start : i+1])
				if json.Valid(candidate) {
					return json.RawMessage(candidate), nil
				}
				return nil, errors.New("LLM 输出的 JSON 不合法")
			}
		}
	}
	return nil, errors.New("LLM 输出的 JSON 不完整")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
