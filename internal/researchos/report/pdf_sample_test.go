package report

import (
	"os"
	"strings"
	"testing"
)

// 手工留证用：RESEARCHOS_PDF_SAMPLE=path go test -run TestWriteSamplePDF ./internal/researchos/report/
// 平时跳过，不进 CI。
func TestWriteSamplePDF(t *testing.T) {
	out := os.Getenv("RESEARCHOS_PDF_SAMPLE")
	if out == "" {
		t.Skip("未设置 RESEARCHOS_PDF_SAMPLE，跳过")
	}
	md := "# 表格渲染验证\n\n## 关键数据对比\n\n| 指标 | pgvector | Qdrant |\n|---|---|---|\n| 召回率@10 | 0.98 | 0.97 |\n| 写入吞吐（条/秒） | 8000 | 15000 |\n| 内存占用（1kw 向量） | 12 GB（HNSW m=16） | 7 GB（量化后） |\n| 运维 | 随 PostgreSQL 备份 | 独立集群 |\n\n超长单元格测试：\n\n| 列A | 极长内容 |\n|---|---|\n| x | " + strings.Repeat("这是一个用于验证列宽压缩与自动换行的长单元格。", 6) + " |\n\n## 信息来源\n\n1. [pgvector](https://github.com/pgvector/pgvector) — github.com\n2. [Qdrant](https://qdrant.tech/) — qdrant.tech\n"
	data, err := RenderPDF("样例", md)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, data, 0644); err != nil {
		t.Fatal(err)
	}
}
