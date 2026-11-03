package secrets

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKey() []byte {
	k := make([]byte, KeySize)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestSealOpenRoundTrip(t *testing.T) {
	box, err := NewBox(testKey())
	if err != nil {
		t.Fatal(err)
	}
	plain := "sk-proj-abcdefghijklmnopqrstuvwxyz012345"
	ct, err := box.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncrypted(ct) {
		t.Fatalf("密文应带前缀，实际 %q", ct)
	}
	if strings.Contains(ct, plain) {
		t.Fatal("密文里不应出现明文")
	}
	got, err := box.Open(ct)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("解密结果不一致：%q", got)
	}
}

// 同一个明文两次加密必须得到不同密文（nonce 随机），否则相同 Key 会暴露「两处配置一样」。
func TestSealIsNonDeterministic(t *testing.T) {
	box, _ := NewBox(testKey())
	a, _ := box.Seal("same-value")
	b, _ := box.Seal("same-value")
	if a == b {
		t.Fatal("两次加密结果不应相同")
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	box, _ := NewBox(testKey())
	ct, _ := box.Seal("secret-value")
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(ct, Prefix))
	raw[len(raw)-1] ^= 0xFF
	tampered := Prefix + base64.StdEncoding.EncodeToString(raw)
	if _, err := box.Open(tampered); err == nil {
		t.Fatal("篡改后的密文必须解密失败")
	}
}

func TestOpenWithWrongKeyFails(t *testing.T) {
	box, _ := NewBox(testKey())
	ct, _ := box.Seal("secret-value")

	other := testKey()
	other[0] ^= 0xFF
	box2, _ := NewBox(other)
	if _, err := box2.Open(ct); err == nil {
		t.Fatal("换一把密钥必须解密失败")
	}
}

// 历史明文（改造成加密存储之前写进去的值）要能读出来，否则升级即丢配置。
func TestOpenPassesThroughLegacyPlaintext(t *testing.T) {
	box, _ := NewBox(testKey())
	got, err := box.Open("legacy-plain-value")
	if err != nil {
		t.Fatal(err)
	}
	if got != "legacy-plain-value" {
		t.Fatalf("明文应原样返回，实际 %q", got)
	}
}

func TestParseKeyFormats(t *testing.T) {
	key := testKey()
	cases := map[string]string{
		"base64":     base64.StdEncoding.EncodeToString(key),
		"raw-base64": base64.RawStdEncoding.EncodeToString(key),
		"url-base64": base64.URLEncoding.EncodeToString(key),
		"hex":        strings.ToUpper(hexOf(key)),
	}
	for name, s := range cases {
		got, err := ParseKey(s)
		if err != nil {
			t.Fatalf("%s 解析失败: %v", name, err)
		}
		if len(got) != KeySize {
			t.Fatalf("%s 长度不对: %d", name, len(got))
		}
	}
	if _, err := ParseKey("too-short"); err == nil {
		t.Fatal("长度不足应报错")
	}
	if _, err := ParseKey("   "); err == nil {
		t.Fatal("空值应报错")
	}
}

func hexOf(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}

func TestLoadOrCreateKeyPersistsAndRestrictsPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", KeyFileName)

	k1, created, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("首次调用应生成新密钥")
	}
	k2, created2, err := LoadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if created2 {
		t.Fatal("第二次调用不应重新生成")
	}
	if string(k1) != string(k2) {
		t.Fatal("两次读取的密钥应一致")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows 上 Go 的 FileMode 权限位不生效，只在类 Unix 断言。
	if os.PathSeparator == '/' {
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Fatalf("密钥文件权限过宽: %o", perm)
		}
	}
}

func TestFromEnvOrFilePrefersEnv(t *testing.T) {
	dir := t.TempDir()
	envKey := base64.StdEncoding.EncodeToString(testKey())
	box, source, err := FromEnvOrFile(envKey, filepath.Join(dir, KeyFileName))
	if err != nil {
		t.Fatal(err)
	}
	if box == nil || !strings.HasPrefix(source, "env:") {
		t.Fatalf("应优先使用环境变量，实际来源 %q", source)
	}
}

func TestMaskNeverLeaksMiddle(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"short", "****"},
		{"12345678901", "****"},
		{"sk-abcdefghijklmnop", "sk-a****mnop"},
	}
	for _, c := range cases {
		if got := Mask(c.in); got != c.want {
			t.Fatalf("Mask(%q)=%q，期望 %q", c.in, got, c.want)
		}
	}
	// 中间段绝不能出现在掩码里。
	if strings.Contains(Mask("sk-abcdefghijklmnop"), "efghijkl") {
		t.Fatal("掩码泄露了中间段")
	}
}
