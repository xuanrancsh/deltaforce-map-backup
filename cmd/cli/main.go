// Package main 是 dfmap-cli 命令行入口，供自动化测试与脚本调用。
// 约定：stdout 只输出一行 JSON；日志与调试信息一律走 stderr。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"deltamapbackup/internal/core"
)

// cliOutput 是 CLI 输出到 stdout 的 JSON 结构。
// 字段顺序与契约一致；数组字段始终为非 nil（序列化为 [] 而非 null）。
type cliOutput struct {
	OK        bool     `json:"ok"`
	Command   string   `json:"command"`
	GamePath  string   `json:"game_path"`
	PaksDir   string   `json:"paks_dir"`
	BackupDir string   `json:"backup_dir"`
	InPaks    []string `json:"in_paks"`
	InBackup  []string `json:"in_backup"`
	Moved     []string `json:"moved"`
	Skipped   []string `json:"skipped"`
	Missing   []string `json:"missing"`
	Errors    []string `json:"errors"`
	Bytes     int64    `json:"bytes"`
	Message   string   `json:"message"`
	Source    string   `json:"source,omitempty"`
}

func main() {
	os.Exit(realMain())
}

func realMain() int {
	args := os.Args[1:]
	if len(args) == 0 {
		emit(cliOutput{
			OK:      false,
			Message: "缺少命令参数，请使用 find / status / backup / restore 之一",
		})
		return 1
	}

	cmd := args[0]
	fs := flag.NewFlagSet("dfmap-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		pathFlag     string
		configFlag   string
		conflictFlag string
		dryRun       bool
		jsonFlag     bool
	)
	fs.StringVar(&pathFlag, "path", "", "游戏目录（可选）")
	fs.StringVar(&configFlag, "config", "", "配置文件路径（可用环境变量 DFMAP_CONFIG 覆盖）")
	fs.StringVar(&conflictFlag, "conflict", "ask", "冲突策略：ask|overwrite|skip")
	fs.BoolVar(&dryRun, "dry-run", false, "只报告将要做什么，不真正移动文件")
	fs.BoolVar(&jsonFlag, "json", false, "以 JSON 输出（默认即为 JSON）")

	if err := fs.Parse(args[1:]); err != nil {
		emit(cliOutput{OK: false, Command: cmd, Message: "参数解析失败：" + err.Error()})
		return 1
	}

	if configFlag != "" {
		_ = os.Setenv("DFMAP_CONFIG", configFlag)
	}
	_ = core.InitLog()
	core.Logf("CLI 命令：%s", cmd)

	pol := core.ConflictSkip
	switch strings.ToLower(strings.TrimSpace(conflictFlag)) {
	case "overwrite":
		pol = core.ConflictOverwrite
	case "skip":
		pol = core.ConflictSkip
	default: // ask 及未知值
		if strings.EqualFold(conflictFlag, "ask") {
			core.Logf("CLI 模式下 --conflict ask 等价于 skip")
		}
		pol = core.ConflictSkip
	}

	switch cmd {
	case "find":
		return doFind()
	case "status":
		return doStatus(pathFlag)
	case "backup":
		return doBackup(pathFlag, pol, dryRun)
	case "restore":
		return doRestore(pathFlag, pol, dryRun)
	default:
		emit(cliOutput{OK: false, Command: cmd, Message: "未知命令：" + cmd})
		return 1
	}
}

// doFind 执行自动搜索并保存配置。
func doFind() int {
	root, source, err := core.AutoFind()
	if err != nil {
		emit(cliOutput{OK: false, Command: "find", Message: "自动查找失败：" + err.Error()})
		return 1
	}

	cfg, _ := core.LoadConfig()
	if cfg.BackupDirName == "" {
		cfg.BackupDirName = core.BackupDirName
	}
	gi, nerr := core.NormalizeDirWithBackup(root, cfg.BackupDirName)
	if nerr != nil {
		emit(cliOutput{OK: false, Command: "find", Message: "自动查找结果校验失败：" + nerr.Error()})
		return 1
	}
	cfg.GamePath = gi.Root
	if serr := core.SaveConfig(cfg); serr != nil {
		core.Logf("警告：保存配置失败：%v", serr)
	}

	emit(cliOutput{
		OK:        true,
		Command:   "find",
		GamePath:  gi.Root,
		PaksDir:   gi.Paks,
		BackupDir: gi.Backup,
		InPaks:    listPresent(gi.Paks),
		InBackup:  listPresent(gi.Backup),
		Message:   fmt.Sprintf("已找到游戏目录（来源：%s）：%s", source, gi.Root),
		Source:    source,
	})
	return 0
}

// doStatus 展示当前解析结果与两侧文件情况。
func doStatus(pathFlag string) int {
	gi, err := resolveGame(pathFlag)
	if err != nil {
		emit(cliOutput{OK: false, Command: "status", Message: err.Error()})
		return 1
	}
	inPaks := listPresent(gi.Paks)
	inBackup := listPresent(gi.Backup)
	emit(cliOutput{
		OK:        true,
		Command:   "status",
		GamePath:  gi.Root,
		PaksDir:   gi.Paks,
		BackupDir: gi.Backup,
		InPaks:    inPaks,
		InBackup:  inBackup,
		Message:   fmt.Sprintf("Paks 目录有 %d 个待备份文件，备份夹有 %d 个文件。", len(inPaks), len(inBackup)),
	})
	return 0
}

// doBackup 执行备份。
func doBackup(pathFlag string, pol core.ConflictPolicy, dryRun bool) int {
	gi, err := resolveGame(pathFlag)
	if err != nil {
		emit(cliOutput{OK: false, Command: "backup", Message: err.Error()})
		return 1
	}
	if dryRun {
		return doDryRun("backup", gi)
	}

	res, err := core.Backup(gi, pol, nil)
	out := resultToOutput("backup", gi, res)
	if err != nil {
		out.OK = false
		if out.Message == "" {
			out.Message = err.Error()
		}
	}
	emit(out)
	if err != nil || !res.OK {
		return 1
	}
	return 0
}

// doRestore 执行恢复。
func doRestore(pathFlag string, pol core.ConflictPolicy, dryRun bool) int {
	gi, err := resolveGame(pathFlag)
	if err != nil {
		emit(cliOutput{OK: false, Command: "restore", Message: err.Error()})
		return 1
	}
	if dryRun {
		return doDryRun("restore", gi)
	}

	res, err := core.Restore(gi, pol, nil)
	out := resultToOutput("restore", gi, res)
	if err != nil {
		out.OK = false
		if out.Message == "" {
			out.Message = err.Error()
		}
	}
	emit(out)
	if err != nil || !res.OK {
		return 1
	}
	return 0
}

// doDryRun 只报告将要进行的操作，不真正移动文件。
func doDryRun(cmd string, gi core.GameInfo) int {
	var moved, skipped []string
	srcDir := gi.Paks
	if cmd == "restore" {
		srcDir = gi.Backup
	}
	for _, name := range core.TargetFiles {
		if _, err := os.Stat(filepath.Join(srcDir, name)); err != nil {
			skipped = append(skipped, name+"（源位置不存在该文件）")
			continue
		}
		moved = append(moved, name)
	}
	action := "备份"
	if cmd == "restore" {
		action = "恢复"
	}
	emit(cliOutput{
		OK:        true,
		Command:   cmd,
		GamePath:  gi.Root,
		PaksDir:   gi.Paks,
		BackupDir: gi.Backup,
		InPaks:    listPresent(gi.Paks),
		InBackup:  listPresent(gi.Backup),
		Moved:     moved,
		Skipped:   skipped,
		Message:   fmt.Sprintf("dry-run：%s 时将移动 %d 个文件，跳过 %d 个（未真正执行）。", action, len(moved), len(skipped)),
	})
	return 0
}

// resolveGame 解析游戏目录：优先 --path，其次已保存配置，最后自动查找。
func resolveGame(pathFlag string) (core.GameInfo, error) {
	cfg, _ := core.LoadConfig()
	backupName := cfg.BackupDirName
	if backupName == "" {
		backupName = core.BackupDirName
	}

	if strings.TrimSpace(pathFlag) != "" {
		return core.NormalizeDirWithBackup(pathFlag, backupName)
	}
	if strings.TrimSpace(cfg.GamePath) != "" {
		if gi, err := core.NormalizeDirWithBackup(cfg.GamePath, backupName); err == nil {
			return gi, nil
		}
	}
	root, _, err := core.AutoFind()
	if err != nil {
		return core.GameInfo{}, err
	}
	return core.NormalizeDirWithBackup(root, backupName)
}

// resultToOutput 把 Result 合并路径字段后转换为 CLI 输出。
func resultToOutput(cmd string, gi core.GameInfo, res core.Result) cliOutput {
	return cliOutput{
		OK:        res.OK,
		Command:   cmd,
		GamePath:  gi.Root,
		PaksDir:   gi.Paks,
		BackupDir: gi.Backup,
		InPaks:    listPresent(gi.Paks),
		InBackup:  listPresent(gi.Backup),
		Moved:     ensureSlice(res.Moved),
		Skipped:   ensureSlice(res.Skipped),
		Missing:   ensureSlice(res.Missing),
		Errors:    ensureSlice(res.Errors),
		Bytes:     res.Bytes,
		Message:   buildMessage(cmd, res),
	}
}

// buildMessage 依据结果生成一句中文说明。
func buildMessage(cmd string, res core.Result) string {
	action := "备份"
	if cmd == "restore" {
		action = "恢复"
	}
	if !res.OK {
		if len(res.Errors) > 0 {
			return action + "失败：" + strings.Join(res.Errors, "；")
		}
		return action + "失败。"
	}
	return fmt.Sprintf("%s完成：已移动 %d 个文件，跳过 %d 个，共 %s。",
		action, len(res.Moved), len(res.Skipped), formatBytes(res.Bytes))
}

// listPresent 列出某目录下存在的目标文件（保持 TargetFiles 顺序）。
func listPresent(dir string) []string {
	out := []string{}
	if strings.TrimSpace(dir) == "" {
		return out
	}
	for _, name := range core.TargetFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			out = append(out, name)
		}
	}
	return out
}

// ensureSlice 保证返回非 nil 切片。
func ensureSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
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

// emit 把输出结构补齐空数组后序列化为一行 JSON 写到 stdout。
func emit(out cliOutput) {
	out.InPaks = ensureSlice(out.InPaks)
	out.InBackup = ensureSlice(out.InBackup)
	out.Moved = ensureSlice(out.Moved)
	out.Skipped = ensureSlice(out.Skipped)
	out.Missing = ensureSlice(out.Missing)
	out.Errors = ensureSlice(out.Errors)

	data, err := json.Marshal(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "JSON 序列化失败："+err.Error())
		fmt.Fprintln(os.Stdout,
			`{"ok":false,"command":"","game_path":"","paks_dir":"","backup_dir":"","in_paks":[],"in_backup":[],"moved":[],"skipped":[],"missing":[],"errors":[],"bytes":0,"message":"JSON 序列化失败"}`)
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}
