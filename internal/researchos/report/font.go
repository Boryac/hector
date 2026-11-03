package report

import (
	_ "embed"
	"fmt"
	"os"
	"sort"
)

// 内嵌字体：Noto Sans SC Regular（SIL OFL 1.1，授权全文见 fonts/OFL.txt）。
//
// 为什么内嵌而不是运行时扫描系统字体：
//   - PDF 导出的中文字形必须随处可得——Docker/alpine 镜像默认没有任何 CJK 字体，
//     Windows 有 simhei 但 Linux 没有，靠系统字体等于把部署环境变成功能开关；
//   - gopdf 只支持 glyf 型 TTF，主流 Linux 发行版的 CJK 字体几乎全是 TTC/OTF（CFF），
//     运行时找得到也读不了；
//   - gopdf 在写 PDF 时只嵌入实际用到的字形，产出文件只有几十 KB，仓库成本是唯一代价。
//
// 字体制备（换字体时重跑）：
//  1. 从 google/fonts 下载 NotoSansSC[wght].ttf；
//  2. fontTools instancer 静态化 wght=400，修正 name/OS-2；
//  3. python scripts/gen_font_coverage.py 重新生成覆盖区间表。
//
//go:embed fonts/NotoSansSC-Regular.ttf
var embeddedFont []byte

// fallbackRune 是字体覆盖不到的码点的替身（U+25A1 WHITE SQUARE，字体里有）。
const fallbackRune = '□'

// fontCoverage 二分查找：码点是否被字体覆盖。
func coveredByFont(r rune) bool {
	lo, hi := 0, len(fontCoverage)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case r < fontCoverage[mid][0]:
			hi = mid
		case r > fontCoverage[mid][1]:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// sanitizeForFont 把字体覆盖不到的码点替换为 '□'，保证排版不会中途报错。
// 返回替换数量；正常中文/英文报告应当为 0。
func sanitizeForFont(s string) (string, int) {
	replaced := 0
	runes := []rune(s)
	for i, r := range runes {
		if r == '\n' || r == '\t' || coveredByFont(r) {
			continue
		}
		runes[i] = fallbackRune
		replaced++
	}
	return string(runes), replaced
}

// fontBytes 解析要使用的字体字节：优先 RESEARCHOS_PDF_FONT 指定的 TTF 文件，
// 失败或未设置时回落到内嵌字体。允许指定外部字体是为了换品牌字重（比如加粗标题体），
// 文件必须是 glyf 型 TTF；TTC / OTF(CFF) 不受支持，会得到明确的错误而不是坏文件。
func fontBytes() ([]byte, string, error) {
	if p := os.Getenv("RESEARCHOS_PDF_FONT"); p != "" {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, "", fmt.Errorf("读取 RESEARCHOS_PDF_FONT 失败: %w", err)
		}
		if len(data) > 12 && string(data[:4]) != "\x00\x01\x00\x00" && string(data[:4]) != "true" {
			return nil, "", fmt.Errorf("%s 不是 glyf 型 TTF（magic=%q）；TTC/OTF 不受支持", p, string(data[:4]))
		}
		return data, "file:" + p, nil
	}
	return embeddedFont, "embedded", nil
}

// coverageBounds 返回覆盖区间数（测试与诊断用）。
func coverageBounds() int { return len(fontCoverage) }

// sortRanges 确保区间有序（生成器保证有序，这里防御外字体校准脚本出错）。
func sortRanges() {
	sort.Slice(fontCoverage, func(i, j int) bool {
		return fontCoverage[i][0] < fontCoverage[j][0]
	})
}
