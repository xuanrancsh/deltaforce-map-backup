package main

import (
	"os"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"golang.org/x/image/font/sfnt"
)

// fontCandidates 是系统内置中文字体文件的候选列表（按优先级排序）。
// Deng.ttf（等线）为 Win10+ 自带，优先使用。
var fontCandidates = []string{
	`C:\Windows\Fonts\Deng.ttf`,
	`C:\Windows\Fonts\msyh.ttc`,
	`C:\Windows\Fonts\simhei.ttf`,
	`C:\Windows\Fonts\simkai.ttf`,
	`C:\Windows\Fonts\simfang.ttf`,
	`C:\Windows\Fonts\simsun.ttc`,
}

var (
	fontOnce    sync.Once
	chineseFont fyne.Resource
)

// installChineseFont 尝试加载系统中文字体。
// 可用环境变量 DFMAP_FONT 指定字体文件路径（优先于内置候选）。
// 失败时不 panic，仅使 chineseFont 保持为 nil。
func installChineseFont() {
	fontOnce.Do(func() {
		paths := make([]string, 0, len(fontCandidates)+1)
		if env := os.Getenv("DFMAP_FONT"); env != "" {
			paths = append(paths, env)
		}
		paths = append(paths, fontCandidates...)

		for _, p := range paths {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			if !validChineseFont(data) {
				continue
			}
			chineseFont = fyne.NewStaticResource("zh.ttf", data)
			return
		}
	})
}

// validChineseFont 校验字体数据能否被解析，且包含中文字形「中」。
// 这样 .ttc 等不受 sfnt.Parse 支持的格式会被自动跳过，而不会导致崩溃。
func validChineseFont(data []byte) bool {
	f, err := sfnt.Parse(data)
	if err != nil {
		return false
	}
	var buf sfnt.Buffer
	idx, err := f.GlyphIndex(&buf, '中')
	if err != nil {
		return false
	}
	return idx > 0
}

// zhTheme 是基于默认主题、仅替换字体的自定义主题。
type zhTheme struct {
	fyne.Theme
	base fyne.Theme
	font fyne.Resource
}

// Font 返回中文字体资源；字体为空时回退到基础主题。
func (t *zhTheme) Font(s fyne.TextStyle) fyne.Resource {
	if t.font != nil {
		return t.font
	}
	return t.base.Font(s)
}

// applyChineseTheme 应用自定义中文主题；没有可用字体时返回 false。
func applyChineseTheme(a fyne.App) bool {
	if chineseFont == nil {
		return false
	}
	base := theme.DefaultTheme()
	a.Settings().SetTheme(&zhTheme{Theme: base, base: base, font: chineseFont})
	return true
}
