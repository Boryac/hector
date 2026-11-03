package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadEnvFile 读取 KEY=VALUE 形式的本地配置文件（默认 .env.researchos）。
//
// 规则：
//   - 空行与以 # 开头的行忽略；
//   - 已存在的环境变量优先，不被文件覆盖（便于用 shell 临时覆盖）；
//   - 值两端的成对引号会被剥离（支持带空格的 DSN）；
//   - 文件不存在不算错误，返回 os.ErrNotExist 由调用方决定是否忽略。
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 8*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 {
			if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		if k == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); exists {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return sc.Err()
}
