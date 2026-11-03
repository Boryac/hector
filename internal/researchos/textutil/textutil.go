// Package textutil 提供文本与 URL 的规范化、正文抽取与关键词切分。
//
// 这些函数是「资料 → 证据」链路的底座：抽取质量直接决定结论质量，
// 因此所有函数都要对中文与英文同时成立，且不得把页面框架文本当成正文。
package textutil

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strings"
)

// NormalizeURL 规范化 URL：去 utm 系列参数、去 fragment、去尾部斜杠、小写化 scheme 与 host。
//
// 规范化结果是去重键（sources.normalized_url），因此必须稳定：
// 同一页面无论带不带跟踪参数都应得到同一个值。
func NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("空 URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", schemeError{}
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return "", errors.New("URL 缺少主机名")
	}
	// 去掉默认端口，避免 example.com 与 example.com:443 被当成两条。
	host = strings.TrimSuffix(strings.TrimSuffix(host, ":443"), ":80")

	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	// 去尾部斜杠（根路径除外）。
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "/"
		}
	}

	q := u.Query()
	for k := range q {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "utm_") || lk == "ref" || lk == "fbclid" || lk == "gclid" || lk == "spm" {
			q.Del(k)
		}
	}
	// 参数排序由 url.Values.Encode 保证（按键名排序），结果稳定。
	query := q.Encode()

	out := scheme + "://" + host + path
	if query != "" {
		out += "?" + query
	}
	if out == scheme+"://"+host+"/" {
		out = scheme + "://" + host
	}
	return out, nil
}

type schemeError struct{}

func (schemeError) Error() string { return "unsupported scheme" }

// Host 返回去掉 www. 的小写主机名，解析失败返回空串。
func Host(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	h := strings.ToLower(u.Hostname())
	return strings.TrimPrefix(h, "www.")
}

// Hash 返回内容指纹。
func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

var (
	scriptRe  = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe   = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscript  = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	tagRe     = regexp.MustCompile(`(?s)<[^>]*>`)
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaRe    = regexp.MustCompile(`(?is)<meta[^>]+(?:name|property)=["'](?:description|og:description)["'][^>]*content=["']([^"']*)["']`)
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	spaceRe   = regexp.MustCompile(`\s+`)
	navBlock  = regexp.MustCompile(`(?is)<(nav|footer|aside)[^>]*>.*?</(nav|footer|aside)>`)
)

var (
	mainOpenRe   = regexp.MustCompile(`(?is)<main[^>]*>`)
	articleOpen  = regexp.MustCompile(`(?is)<article[^>]*>`)
	mainCloseRe  = regexp.MustCompile(`(?is)</main>`)
	articleClose = regexp.MustCompile(`(?is)</article>`)
	wikiLinkRe   = regexp.MustCompile(`\[\[([^\]|]+)\|([^\]]+)\]\]`)
	wikiBareRe   = regexp.MustCompile(`\[\[([^\]]+)\]\]`)
	wikiEmphRe   = regexp.MustCompile(`'{2,5}([^']+)'{2,5}`)
	wikiTplRe    = regexp.MustCompile(`(?s)\{\{[^{}]*\}\}`)
	wikiTblRe    = regexp.MustCompile(`(?s)\{\|.*?\|\}`)
)

// boilerplate 是页面框架噪声特征，命中即丢弃该行。
var boilerplate = []string{
	"toggle", "jump to", "navigation menu", "retrieved from", "privacy policy",
	"cookie policy", "terms of use", "subscribe", "sign in", "log in", "advertisement",
	"share this", "follow us", "all rights reserved", "skip to content", "edit links",
	"Wikimedia Foundation", "Powered by", "相关内容", "登录", "注册", "版权所有",
	// 维基/文档站框架噪声（这些行不含任何事实，抽出来会变成垃圾结论）。
	"learn how and when to remove", "switch to legacy parser", "from wikipedia",
	"jump to content", "main page", "current events", "random article",
	"donate to wikipedia", "wikimedia commons", "upload file", "special pages",
	"printable version", "download as pdf", "what links here", "related changes",
	"permanent link", "page information", "cite this page", "text is available under",
	"additional terms may apply", "is a registered trademark",
	// GitHub / 代码托管站页面框架（repo 页的导航块会污染正文）。
	"repository navigation", "security and quality", "provide feedback",
	"save thousands of hours", "terms privacy security status docs",
	"contact github", "pricing api training blog about", "you signed in with another tab",
}

// mainRegion 尽量截取正文区域（<main> / <article>），避免导航栏与页脚噪声。
func mainRegion(doc string) string {
	if loc := mainOpenRe.FindStringIndex(doc); loc != nil {
		rest := doc[loc[1]:]
		if end := mainCloseRe.FindStringIndex(rest); end != nil {
			return rest[:end[0]]
		}
		return rest
	}
	if loc := articleOpen.FindStringIndex(doc); loc != nil {
		rest := doc[loc[1]:]
		if end := articleClose.FindStringIndex(rest); end != nil {
			return rest[:end[0]]
		}
		return rest
	}
	return doc
}

// cleanMarkup 清理维基/文档站常见轻量标记。
func cleanMarkup(s string) string {
	// 嵌套模板逐层剥离（最多 3 层，避免病态输入死循环）。
	for i := 0; i < 3; i++ {
		next := wikiTplRe.ReplaceAllString(s, " ")
		if next == s {
			break
		}
		s = next
	}
	s = wikiTblRe.ReplaceAllString(s, " ")
	s = wikiLinkRe.ReplaceAllString(s, "$2")
	s = wikiBareRe.ReplaceAllString(s, "$1")
	s = wikiEmphRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "[edit]", "")
	return s
}

func looksLikeBoilerplate(lower string) bool {
	for _, b := range boilerplate {
		if strings.Contains(lower, strings.ToLower(b)) {
			return true
		}
	}
	return false
}

// isMaintenanceNote 判断整行是否是维基维护标记，例如
// "(December 2025)"、"( Learn how and when to remove this message )"。
func isMaintenanceNote(line string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, "(") || !strings.HasSuffix(t, ")") {
		return false
	}
	inner := strings.TrimSpace(t[1 : len(t)-1])
	if inner == "" || len([]rune(inner)) > 80 {
		return false
	}
	lower := strings.ToLower(inner)
	for _, kw := range []string{"remove", "cleanup", "citation", "expand", "update", "disputed", "verify", "reliable"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// 仅由「月份 + 年份」组成的日期括注也属于维护标记。
	hasMonth := false
	for _, m := range []string{"january", "february", "march", "april", "may", "june",
		"july", "august", "september", "october", "november", "december"} {
		if strings.Contains(lower, m) {
			hasMonth = true
			break
		}
	}
	if hasMonth {
		for _, r := range lower {
			if r >= '0' && r <= '9' {
				return true
			}
		}
	}
	return false
}

// referenceLitter 判断整行是否只是引用编号堆叠（"[ 1 ] [ 8 ]"、"[edit]" 等）。
func referenceLitter(line string) bool {
	if strings.Count(line, "[") < 2 {
		return false
	}
	letters := 0
	for _, r := range line {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= 0x4e00 && r <= 0x9fff {
			letters++
		}
	}
	return strings.Count(line, "[")+strings.Count(line, "]") >= letters/3
}

// ExtractHTML 抽取标题与正文纯文本。
func ExtractHTML(doc string) (title, text string) {
	doc = commentRe.ReplaceAllString(doc, " ")
	doc = navBlock.ReplaceAllString(doc, " ")
	if m := titleRe.FindStringSubmatch(doc); len(m) > 1 {
		title = CleanText(html.UnescapeString(m[1]))
	}
	if title == "" {
		if m := metaRe.FindStringSubmatch(doc); len(m) > 1 {
			title = CleanText(html.UnescapeString(m[1]))
			if runes := []rune(title); len(runes) > 200 {
				title = string(runes[:200])
			}
		}
	}
	s := mainRegion(doc)
	s = scriptRe.ReplaceAllString(s, " ")
	s = styleRe.ReplaceAllString(s, " ")
	s = noscript.ReplaceAllString(s, " ")
	// 块级标签转换为换行，避免句子粘连。
	for _, t := range []string{"</p>", "</div>", "</li>", "</h1>", "</h2>", "</h3>", "</h4>", "<br", "</tr>", "</section>", "</article>"} {
		s = strings.ReplaceAll(s, t, "\n")
	}
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = cleanMarkup(s)
	lines := strings.Split(s, "\n")
	kept := make([]string, 0, len(lines))
	for _, l := range lines {
		l = CleanText(l)
		if len([]rune(l)) < 20 {
			continue
		}
		lower := strings.ToLower(l)
		if looksLikeBoilerplate(lower) {
			continue
		}
		if isMaintenanceNote(l) || referenceLitter(l) {
			continue
		}
		// 丢弃以标点/数字为主的噪声行（导航计数、表格残余）。
		letters := 0
		for _, r := range l {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= 0x4e00 && r <= 0x9fff {
				letters++
			}
		}
		if float64(letters)/float64(len([]rune(l))) < 0.45 {
			continue
		}
		kept = append(kept, l)
	}
	return title, strings.Join(kept, "\n")
}

// CleanText 压缩空白并去除不可见字符。
func CleanText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\u200b", "")
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// Excerpt 从正文中截取包含关键词的最佳片段。
//
// 全程使用 rune 下标：中文一个字符占 3 字节，混用字节下标会算出超长片段。
func Excerpt(text string, terms []string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 600
	}
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	lowerRunes := []rune(strings.ToLower(text))
	best := -1
	for _, t := range terms {
		t = strings.ToLower(strings.TrimSpace(t))
		if len([]rune(t)) < 2 {
			continue
		}
		idx := indexRunes(lowerRunes, []rune(t))
		if idx >= 0 && (best < 0 || idx < best) {
			best = idx
		}
	}
	// 预留 2 个 rune 给两侧省略号，保证输出不超过 maxLen。
	budget := maxLen - 2
	if budget < 8 {
		budget = maxLen
	}
	start := 0
	if best > 0 {
		start = best - budget/4
		if start < 0 {
			start = 0
		}
	}
	if start+budget > len(runes) {
		start = len(runes) - budget
		if start < 0 {
			start = 0
		}
	}
	end := start + budget
	if end > len(runes) {
		end = len(runes)
	}
	out := string(runes[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return CleanText(out)
}

// indexRunes 在 rune 切片中查找子串，返回 rune 下标（未命中返回 -1）。
func indexRunes(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// Sentence 返回包含关键词的第一句（用于证据引文）。
func Sentence(text string, terms []string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 300
	}
	lower := strings.ToLower(text)
	runes := []rune(text)
	isStop := func(r rune) bool {
		switch r {
		case '.', '!', '?', '\n', '。', '！', '？', '；', ';':
			return true
		}
		return false
	}
	for _, t := range terms {
		t = strings.ToLower(strings.TrimSpace(t))
		if len([]rune(t)) < 2 {
			continue
		}
		idx := strings.Index(lower, t)
		if idx < 0 {
			continue
		}
		// 把字节偏移换算成 rune 偏移。
		startRune := len([]rune(text[:idx]))
		begin := startRune
		for begin > 0 && !isStop(runes[begin-1]) {
			begin--
		}
		end := startRune
		for end < len(runes) && !isStop(runes[end]) {
			end++
		}
		reachedEOF := end >= len(runes)
		if end < len(runes) {
			end++
		}
		s := CleanText(string(runes[begin:end]))
		if r := []rune(s); len(r) > maxLen {
			s = string(r[:maxLen]) + "…"
		} else if reachedEOF && !isStop(lastRune(s)) {
			// 已到文本末尾且没有句末标点：补句号，避免结论以逗号/单词中断结尾。
			s += "。"
		}
		if n := len([]rune(s)); n >= 6 {
			return s
		} else if n >= 4 && isStop(lastRune(s)) {
			return s
		}
	}
	return ""
}

// Terms 把查询串切成关键词（英文按非字母数字切分，中文整段保留）。
func Terms(q string) []string {
	q = strings.ToLower(q)
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && !(r >= 0x4e00 && r <= 0x9fff)
	})
	out := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, f := range fields {
		if len([]rune(f)) < 2 || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// WordCount 统计中英文词数（中文按字数，英文按词计）。
func WordCount(s string) int {
	cn := 0
	for _, r := range s {
		if r >= 0x4e00 && r <= 0x9fff {
			cn++
		}
	}
	ascii := 0
	for _, f := range strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
	}) {
		if f != "" {
			ascii++
		}
	}
	return cn + ascii
}

// lastRune 返回字符串最后一个 rune（空串返回 0）。
func lastRune(s string) rune {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}
