// Package main 是「三角洲地图文件备份与恢复」的图形界面入口。
package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"deltamapbackup/internal/core"
	"deltamapbackup/internal/ui"
)

//go:embed assets/icon.png
var iconPNG []byte

func main() {
	// 先初始化日志，保证后续任何一步出问题都有记录。
	_ = core.InitLog()
	core.Logf("程序启动（三角洲地图文件备份与恢复）")

	// 在 app.New() 之前载入中文字体数据，避免界面中文显示为方块。
	installChineseFont()

	a := app.NewWithID("com.deltamapbackup.app")
	if applyChineseTheme(a) {
		core.Logf("已载入中文字体：" + ChineseFontPath())
	} else {
		core.Logf("未找到中文字体，界面中文可能显示为方块（可用环境变量 DFMAP_FONT 指定字体文件）")
	}
	a.SetIcon(fyne.NewStaticResource("icon.png", iconPNG))

	ui.Run(a)
}
