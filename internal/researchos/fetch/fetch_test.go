package fetch

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestGuardHostBlocksLocalAndPrivate(t *testing.T) {
	blocked := []string{
		"localhost", "app.localhost", "svc.internal",
		"127.0.0.1", "127.1.2.3", "10.0.0.5", "172.16.4.4", "192.168.1.1",
		"169.254.169.254", "0.0.0.0", "::1", "fe80::1",
	}
	for _, h := range blocked {
		if err := guardHost(h); err == nil {
			t.Fatalf("%q 应被拒绝", h)
		}
	}
}

func TestGuardHostAllowsPublic(t *testing.T) {
	for _, h := range []string{"example.com", "en.wikipedia.org", "8.8.8.8"} {
		if err := guardHost(h); err != nil {
			t.Fatalf("%q 应被允许，实际：%v", h, err)
		}
	}
}

func TestGuardHostEmpty(t *testing.T) {
	if err := guardHost(""); err == nil {
		t.Fatal("空主机名应被拒绝")
	}
}

func TestGuardIP(t *testing.T) {
	bad := []string{"127.0.0.1", "10.1.2.3", "192.168.0.1", "169.254.169.254", "0.0.0.0", "::1"}
	for _, s := range bad {
		if err := guardIP(net.ParseIP(s)); err == nil {
			t.Fatalf("%s 应被拒绝", s)
		}
	}
	if err := guardIP(net.ParseIP("93.184.216.34")); err != nil {
		t.Fatalf("公网地址应允许：%v", err)
	}
	if err := guardIP(nil); err == nil {
		t.Fatal("nil 地址应被拒绝")
	}
}

func TestGuardResolvedRejectsLoopbackHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := guardResolved(ctx, "127.0.0.1"); err == nil {
		t.Fatal("解析到回环地址的主机应被拒绝")
	}
}

func TestParseRobotsWildcard(t *testing.T) {
	body := "User-agent: *\nDisallow: /private\nDisallow: /admin*\n"
	r := parseRobots(body, "HectorResearchOS/1.0")
	if len(r.disallow) != 2 {
		t.Fatalf("应解析出 2 条规则，实际 %v", r.disallow)
	}
	if !matchDisallow(r.disallow, "/private/page") {
		t.Fatal("/private/page 应被禁止")
	}
	if matchDisallow(r.disallow, "/public/page") {
		t.Fatal("/public/page 不应被禁止")
	}
}

func TestParseRobotsSpecificAgent(t *testing.T) {
	body := "User-agent: Googlebot\nDisallow: /\n\nUser-agent: HectorResearchOS\nDisallow: /blocked\n"
	r := parseRobots(body, "HectorResearchOS/1.0")
	if matchDisallow(r.disallow, "/anything") {
		t.Fatal("不应套用其他爬虫的规则")
	}
	if !matchDisallow(r.disallow, "/blocked/x") {
		t.Fatal("应套用自身 UA 的规则")
	}
}

func TestParseRobotsGroupSeparatorResets(t *testing.T) {
	// 空行后的 Disallow 属于新分组，缺少 User-agent 声明时不得继承上一组。
	body := "User-agent: *\nDisallow: /a\n\nDisallow: /orphan\n"
	r := parseRobots(body, "HectorResearchOS/1.0")
	if !matchDisallow(r.disallow, "/a/1") {
		t.Fatal("第一组规则应生效")
	}
	if matchDisallow(r.disallow, "/orphan/1") {
		t.Fatal("缺少 User-agent 声明的分组不应生效")
	}
}

func TestParseRobotsIgnoresComments(t *testing.T) {
	body := "# comment line\nUser-agent: * # inline\nDisallow: /x # inline\n"
	r := parseRobots(body, "HectorResearchOS/1.0")
	if len(r.disallow) != 1 || r.disallow[0] != "/x" {
		t.Fatalf("注释应被忽略，实际 %v", r.disallow)
	}
}

func TestParseRobotsDisallowRoot(t *testing.T) {
	r := parseRobots("User-agent: *\nDisallow: /\n", "HectorResearchOS/1.0")
	if !matchDisallow(r.disallow, "/any/path") {
		t.Fatal("Disallow: / 应禁止全站")
	}
}

func TestParseRobotsEmptyDisallowIgnored(t *testing.T) {
	r := parseRobots("User-agent: *\nDisallow:\n", "HectorResearchOS/1.0")
	if len(r.disallow) != 0 {
		t.Fatalf("空 Disallow 表示允许全部，实际 %v", r.disallow)
	}
}

func TestRenderDispatch(t *testing.T) {
	title, text := render("text/html; charset=utf-8", []byte(`<html><head><title>标题</title></head><body><main><p>这是一段足够长的正文内容，用来验证 HTML 抽取路径是否正确工作。</p></main></body></html>`))
	if title != "标题" {
		t.Fatalf("HTML 标题解析失败：%q", title)
	}
	if !strings.Contains(text, "正文内容") {
		t.Fatalf("HTML 正文抽取失败：%q", text)
	}

	title, text = render("application/json", []byte(`{"title":"接口标题","body":"接口正文内容够长的一段文字"}`))
	if title != "接口标题" || !strings.Contains(text, "接口正文") {
		t.Fatalf("JSON 渲染失败：%q / %q", title, text)
	}

	title, text = render("text/plain", []byte("纯文本正文"))
	if title == "" || text != "纯文本正文" {
		t.Fatalf("纯文本渲染失败：%q / %q", title, text)
	}
}
