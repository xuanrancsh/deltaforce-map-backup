// Package ui 提供「三角洲地图文件备份与恢复」的图形界面。
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"deltamapbackup/internal/core"
)

// maxLogLines 是操作记录区保留的最大行数。
const maxLogLines = 200

// App 承载界面控件与运行状态。
type App struct {
	fyneApp fyne.App
	win     fyne.Window

	pathEntry   *widget.Entry
	statusLabel *widget.Label
	logRich     *widget.RichText
	logScroll   *container.Scroll

	findBtn    *widget.Button
	manualBtn  *widget.Button
	backupBtn  *widget.Button
	restoreBtn *widget.Button

	logLines []string
	gi       core.GameInfo

	// 本次运行内是否已由用户在冲突弹窗中勾选「一直这样做」，以及采用的策略。
	conflictResolved bool
	conflictPolicy   core.ConflictPolicy
}

// Run 创建并显示主窗口。
func Run(a fyne.App) {
	u := newApp(a)
	u.win.ShowAndRun()
}

// newApp 构建界面并启动初始化流程。
func newApp(a fyne.App) *App {
	u := &App{fyneApp: a}
	u.win = a.NewWindow("三角洲地图文件备份与恢复")
	u.win.Resize(fyne.NewSize(1040, 740))
	u.win.CenterOnScreen()
	u.buildUI()
	u.startup()
	return u
}

// buildUI 按规划搭建界面布局。
func (u *App) buildUI() {
	// ── 顶部：路径 + 按钮 + 状态 ────────────────────────────────────────────
	u.pathEntry = widget.NewEntry()
	u.pathEntry.Disable()

	u.findBtn = widget.NewButton("自动查找", u.onFind)
	u.manualBtn = widget.NewButton("手动指定…", u.onManual)

	pathRow := container.NewBorder(
		nil, nil,
		widget.NewLabel("游戏安装路径："),
		container.NewHBox(u.findBtn, u.manualBtn),
		u.pathEntry,
	)

	u.statusLabel = widget.NewLabel("正在初始化…")
	top := container.NewVBox(pathRow, u.statusLabel, widget.NewSeparator())

	// ── 中部：左右两栏（备份 / 恢复）────────────────────────────────────────
	leftCard := u.buildBackupCard()
	rightCard := u.buildRestoreCard()
	split := container.NewHSplit(leftCard, rightCard)
	split.SetOffset(0.5)

	// ── 底部：操作记录（改用 RichText，保证文字清晰、可选中、可自动滚到底）──────
	u.logRich = widget.NewRichText()
	u.logRich.Wrapping = fyne.TextWrapWord
	u.logScroll = container.NewVScroll(u.logRich)
	u.logScroll.SetMinSize(fyne.NewSize(0, 170))

	logTitle := widget.NewLabel("操作记录（最新的在最下面；中间的分隔条可以上下拖动来放大本区域）")
	logBox := container.NewBorder(logTitle, nil, nil, nil, u.logScroll)

	// 主区与日志区之间用可拖动的分隔条，用户可以自己决定日志区多大。
	body := container.NewVSplit(split, logBox)
	body.SetOffset(0.72)

	u.win.SetContent(container.NewBorder(top, nil, nil, nil, body))
}

// buildBackupCard 构建左侧「备份」卡片。
func (u *App) buildBackupCard() fyne.CanvasObject {
	info := widget.NewLabel(
		"把下面 2 个地图文件从 Paks 目录剪切到 MapBackup 备份文件夹：\n\n" +
			"• pak-0-2-pakchunk90-WindowsClient.pak（约 9.1 GB）\n" +
			"• pak-0-2-pakchunk90optional-WindowsClient.pak（约 2.6 GB）\n\n" +
			"备份后游戏内对应地图将无法加载，需要点右侧「恢复」才能还原。")
	info.Wrapping = fyne.TextWrapWord

	u.backupBtn = widget.NewButton("开始备份", u.onBackup)
	u.backupBtn.Importance = widget.HighImportance

	content := container.NewVBox(info, widget.NewSeparator(), u.backupBtn)
	return widget.NewCard("备份", "", content)
}

// buildRestoreCard 构建右侧「恢复」卡片。
func (u *App) buildRestoreCard() fyne.CanvasObject {
	info := widget.NewLabel(
		"把 MapBackup 备份文件夹里的 2 个地图文件剪回 Paks 目录：\n\n" +
			"• pak-0-2-pakchunk90-WindowsClient.pak（约 9.1 GB）\n" +
			"• pak-0-2-pakchunk90optional-WindowsClient.pak（约 2.6 GB）\n\n" +
			"恢复后游戏内对应地图即可正常加载。")
	info.Wrapping = fyne.TextWrapWord

	u.restoreBtn = widget.NewButton("开始恢复", u.onRestore)
	u.restoreBtn.Importance = widget.HighImportance

	content := container.NewVBox(info, widget.NewSeparator(), u.restoreBtn)
	return widget.NewCard("恢复", "", content)
}

// startup 读取配置；有有效路径就直接使用，否则在后台自动查找。
func (u *App) startup() {
	cfg, err := core.LoadConfig()
	if err != nil {
		u.appendLog("读取配置失败：" + err.Error())
	}
	if strings.TrimSpace(cfg.GamePath) != "" {
		if gi, nerr := core.NormalizeDirWithBackup(cfg.GamePath, cfg.BackupDirName); nerr == nil {
			u.setGame(gi)
			u.refreshStatus()
			u.appendLog("已载入上次的游戏目录：" + gi.Root)
			return
		}
	}

	u.appendLog("未找到已保存的有效游戏目录，正在自动查找…")
	u.statusLabel.SetText("正在自动查找游戏目录…")
	go u.autoFind()
}

// autoFind 在后台自动搜索游戏目录，结果通过 fyne.Do 回主线程更新界面。
func (u *App) autoFind() {
	root, source, err := core.AutoFind()
	fyne.Do(func() {
		u.setBusy(false)
		if err != nil {
			u.statusLabel.SetText("未找到游戏目录，请点『手动指定』")
			u.highlightManual(true)
			u.appendLog("自动查找失败：" + err.Error())
			return
		}
		gi, nerr := u.persistPath(root)
		if nerr != nil {
			u.statusLabel.SetText("未找到游戏目录，请点『手动指定』")
			u.highlightManual(true)
			u.appendLog("自动查找结果校验失败：" + nerr.Error())
			return
		}
		u.setGame(gi)
		u.refreshStatus()
		u.appendLog(fmt.Sprintf("自动找到游戏目录（来源：%s）：%s", source, gi.Root))
	})
}

// onFind 处理「自动查找」按钮。
func (u *App) onFind() {
	u.setBusy(true)
	u.statusLabel.SetText("正在自动查找游戏目录…")
	u.appendLog("开始自动查找…")
	go u.autoFind()
}

// onManual 处理「手动指定」按钮。
func (u *App) onManual() {
	u.appendLog("请在弹出的窗口中选中 DeltaForce 游戏文件夹（例如 DeltaForce(2001918) 或它里面的 DeltaForce），也可以直接选 PackContent 或 Paks 目录")
	d := dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, u.win)
			return
		}
		if lu == nil {
			return
		}
		gi, nerr := core.NormalizeDir(lu.Path())
		if nerr != nil {
			dialog.ShowError(nerr, u.win)
			return
		}
		gi2, perr := u.persistPath(gi.Root)
		if perr != nil {
			dialog.ShowError(perr, u.win)
			return
		}
		u.setGame(gi2)
		u.refreshStatus()
		u.appendLog("手动指定游戏目录：" + gi2.Root)
	}, u.win)
	d.Resize(fyne.NewSize(800, 560))
	d.Show()
}

// persistPath 校验并保存游戏路径，成功返回归一化后的 GameInfo。
func (u *App) persistPath(root string) (core.GameInfo, error) {
	cfg, _ := core.LoadConfig()
	cfg.GamePath = root
	if cfg.BackupDirName == "" {
		cfg.BackupDirName = core.BackupDirName
	}
	if err := core.SaveConfig(cfg); err != nil {
		return core.GameInfo{}, err
	}
	return core.NormalizeDirWithBackup(root, cfg.BackupDirName)
}

// setGame 更新当前游戏信息到界面。
func (u *App) setGame(gi core.GameInfo) {
	u.gi = gi
	u.pathEntry.SetText(gi.Root)
	u.highlightManual(false)
}

// refreshStatus 刷新顶部状态行。
func (u *App) refreshStatus() {
	if u.gi.Paks == "" {
		return
	}
	inPaks := countPresent(u.gi.Paks)
	inBackup := countPresent(u.gi.Backup)
	u.statusLabel.SetText(fmt.Sprintf("找到 %d 个待备份文件 · 备份夹内已有 %d 个文件", inPaks, inBackup))
}

// onBackup 处理「开始备份」按钮（先弹确认框）。
func (u *App) onBackup() {
	if u.gi.Paks == "" {
		dialog.ShowInformation("提示", "还没有找到游戏目录，请先点『自动查找』或『手动指定』。", u.win)
		return
	}
	dialog.ShowConfirm("确认备份", u.buildConfirmMessage("backup"), func(ok bool) {
		if !ok {
			return
		}
		if names := u.conflictNames("backup"); len(names) > 0 {
			if u.conflictResolved {
				u.runOp("backup", u.conflictPolicy)
				return
			}
			u.askConflict("backup")
			return
		}
		u.runOp("backup", core.ConflictSkip)
	}, u.win)
}

// onRestore 处理「开始恢复」按钮（先弹确认框）。
func (u *App) onRestore() {
	if u.gi.Paks == "" {
		dialog.ShowInformation("提示", "还没有找到游戏目录，请先点『自动查找』或『手动指定』。", u.win)
		return
	}
	dialog.ShowConfirm("确认恢复", u.buildConfirmMessage("restore"), func(ok bool) {
		if !ok {
			return
		}
		if names := u.conflictNames("restore"); len(names) > 0 {
			if u.conflictResolved {
				u.runOp("restore", u.conflictPolicy)
				return
			}
			u.askConflict("restore")
			return
		}
		u.runOp("restore", core.ConflictSkip)
	}, u.win)
}

// buildConfirmMessage 生成确认框正文，文件大小用实时 os.Stat 结果格式化。
func (u *App) buildConfirmMessage(action string) string {
	var b strings.Builder
	dir := u.gi.Paks
	if action == "restore" {
		dir = u.gi.Backup
		b.WriteString("将把下面 2 个文件从 MapBackup 备份文件夹剪切回 Paks 目录：\n\n")
	} else {
		b.WriteString("将把下面 2 个文件从 Paks 目录剪切到 MapBackup 备份文件夹：\n\n")
	}
	for _, name := range core.TargetFiles {
		b.WriteString(fmt.Sprintf("• %s（%s）\n", name, humanSize(filepath.Join(dir, name))))
	}
	if action == "restore" {
		b.WriteString("\n恢复后游戏内对应地图即可正常加载。\n\n确认继续吗？")
	} else {
		b.WriteString("\n注意：剪切后游戏内对应地图将无法加载，需要点「恢复」才能还原。\n\n确认继续吗？")
	}
	return b.String()
}

// runOp 在后台执行备份 / 恢复，完成后回主线程刷新界面。
func (u *App) runOp(action string, pol core.ConflictPolicy) {
	u.setBusy(true)
	u.appendLog("开始" + opName(action) + "…")

	gi := u.gi
	go func() {
		var res core.Result
		var err error
		if action == "restore" {
			res, err = core.Restore(gi, pol, nil)
		} else {
			res, err = core.Backup(gi, pol, nil)
		}
		fyne.Do(func() {
			u.setBusy(false)
			u.reportResult(res, err)
			u.refreshStatus()
		})
	}()
}

// conflictNames 返回本次操作中「源文件存在且目标位置已存在同名文件」的文件名列表（纯扫描，不弹窗）。
func (u *App) conflictNames(action string) []string {
	srcDir, dstDir := u.gi.Paks, u.gi.Backup
	if action == "restore" {
		srcDir, dstDir = u.gi.Backup, u.gi.Paks
	}
	var names []string
	for _, name := range core.TargetFiles {
		if _, err := os.Stat(filepath.Join(srcDir, name)); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dstDir, name)); err == nil {
			names = append(names, name)
		}
	}
	return names
}

// askConflict 弹一次冲突确认框，用户选完后直接开跑，不做任何阻塞等待。
func (u *App) askConflict(action string) {
	dstDir := u.gi.Backup
	if action == "restore" {
		dstDir = u.gi.Paks
	}
	lines := []string{"目标位置已存在下面的同名文件：", ""}
	for _, n := range u.conflictNames(action) {
		lines = append(lines, "• "+filepath.Join(dstDir, n))
	}
	lines = append(lines, "", "请选择「覆盖」或「跳过」。")

	label := widget.NewLabel(strings.Join(lines, "\n"))
	label.Wrapping = fyne.TextWrapWord
	chk := widget.NewCheck("本次运行内遇到冲突都这样做", func(bool) {})
	box := container.NewVBox(label, chk)

	dialog.NewCustomConfirm("发现同名文件", "覆盖", "跳过", box, func(overwrite bool) {
		pol := core.ConflictSkip
		if overwrite {
			pol = core.ConflictOverwrite
		}
		if chk.Checked {
			u.conflictResolved = true
			u.conflictPolicy = pol
		}
		u.runOp(action, pol)
	}, u.win).Show()
}

// reportResult 把操作结果逐条写入操作记录区。
func (u *App) reportResult(res core.Result, err error) {
	name := opName(res.Action)
	switch {
	case err != nil:
		u.appendLog(fmt.Sprintf("%s失败：%s", name, err.Error()))
	case res.OK:
		u.appendLog(fmt.Sprintf("%s完成。", name))
	default:
		u.appendLog(fmt.Sprintf("%s未能完成。", name))
	}
	for _, m := range res.Moved {
		u.appendLog("  已移动：" + m)
	}
	for _, s := range res.Skipped {
		u.appendLog("  已跳过：" + s)
	}
	for _, m := range res.Missing {
		u.appendLog("  缺失：" + m)
	}
	for _, e := range res.Errors {
		u.appendLog("  失败：" + e)
	}
	if res.Bytes > 0 {
		u.appendLog("  处理数据量：" + formatBytes(res.Bytes))
	}
}

// setBusy 在操作进行中禁用按钮。
func (u *App) setBusy(busy bool) {
	btns := []*widget.Button{u.findBtn, u.manualBtn, u.backupBtn, u.restoreBtn}
	for _, b := range btns {
		if b == nil {
			continue
		}
		if busy {
			b.Disable()
		} else {
			b.Enable()
		}
	}
	if busy {
		u.statusLabel.SetText("正在处理…")
	}
}

// highlightManual 高亮 / 取消高亮「手动指定」按钮。
func (u *App) highlightManual(on bool) {
	if on {
		u.manualBtn.Importance = widget.HighImportance
	} else {
		u.manualBtn.Importance = widget.MediumImportance
	}
	u.manualBtn.Refresh()
}

// appendLog 追加日志到操作记录区，最多保留 maxLogLines 行。
func (u *App) appendLog(line string) {
	stamp := time.Now().Format("15:04:05")
	for _, l := range strings.Split(line, "\n") {
		u.logLines = append(u.logLines, stamp+" "+l)
	}
	if len(u.logLines) > maxLogLines {
		u.logLines = u.logLines[len(u.logLines)-maxLogLines:]
	}
	u.refreshLog()
}

// refreshLog 用最新的 logLines 重建日志区内容，并自动滚动到最底部。
func (u *App) refreshLog() {
	if u.logRich == nil {
		return
	}
	segs := make([]widget.RichTextSegment, 0, len(u.logLines))
	for _, l := range u.logLines {
		segs = append(segs, &widget.TextSegment{
			Text: l,
			Style: widget.RichTextStyle{
				Inline:    false,
				SizeName:  theme.SizeNameText,
				ColorName: theme.ColorNameForeground,
			},
		})
	}
	u.logRich.Segments = segs
	u.logRich.Refresh()
	if u.logScroll != nil {
		u.logScroll.ScrollToBottom()
	}
}

// countPresent 统计某目录下存在的目标文件数量。
func countPresent(dir string) int {
	if dir == "" {
		return 0
	}
	n := 0
	for _, name := range core.TargetFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			n++
		}
	}
	return n
}

// humanSize 返回文件的实时大小描述；取不到时返回「未知大小」。
func humanSize(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return "未知大小"
	}
	return formatBytes(fi.Size())
}

// formatBytes 把字节数格式化为 GB / MB / KB / B。
func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// opName 返回操作的中文名。
func opName(action string) string {
	if action == "restore" {
		return "恢复"
	}
	return "备份"
}
