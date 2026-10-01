package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// ConflictPolicy 表示遇到同名文件时的处理策略。
type ConflictPolicy int

const (
	// ConflictAsk 询问用户。
	ConflictAsk ConflictPolicy = iota
	// ConflictOverwrite 覆盖。
	ConflictOverwrite
	// ConflictSkip 跳过。
	ConflictSkip
)

// ConflictFn 在 ConflictAsk 时被回调，返回本次决策与是否「本次运行内一直这样做」。
type ConflictFn func(fileName string) (ConflictPolicy, bool)

// Result 描述一次备份或恢复的结果，可序列化为 JSON 供 CLI 使用。
type Result struct {
	Action  string   `json:"action"` // "backup" | "restore"
	Moved   []string `json:"moved"`
	Skipped []string `json:"skipped"`
	Missing []string `json:"missing"`
	Errors  []string `json:"errors"`
	Bytes   int64    `json:"bytes"`
	OK      bool     `json:"ok"`
}

// NewResult 构造一个字段全部初始化的结果对象（避免 JSON 里出现 null）。
func NewResult(action string) Result {
	return Result{
		Action:  action,
		Moved:   []string{},
		Skipped: []string{},
		Missing: []string{},
		Errors:  []string{},
		OK:      true,
	}
}

// conflictState 在一次备份 / 恢复过程中记录冲突策略与「本次运行内统一下去」状态。
type conflictState struct {
	policy ConflictPolicy
	locked bool
	ask    ConflictFn
}

// decide 决定当前文件冲突该如何处理。
func (c *conflictState) decide(name string) ConflictPolicy {
	if c.locked {
		return c.policy
	}
	switch c.policy {
	case ConflictOverwrite:
		return ConflictOverwrite
	case ConflictSkip:
		return ConflictSkip
	default:
		if c.ask == nil {
			return ConflictSkip
		}
		p, all := c.ask(name)
		if all {
			c.policy = p
			c.locked = true
		}
		return p
	}
}

// Backup 把目标文件从 Paks 目录「剪切」到备份文件夹。
func Backup(gi GameInfo, pol ConflictPolicy, ask ConflictFn) (Result, error) {
	res := NewResult("backup")
	if gi.Paks == "" {
		msg := "尚未确定游戏目录，无法备份。"
		res.OK = false
		res.Errors = append(res.Errors, msg)
		return res, errors.New(msg)
	}
	if err := os.MkdirAll(gi.Backup, 0755); err != nil {
		msg := "无法创建备份文件夹：" + err.Error()
		res.OK = false
		res.Errors = append(res.Errors, msg)
		return res, err
	}

	cs := &conflictState{policy: pol, ask: ask}
	for _, name := range TargetFiles {
		src := filepath.Join(gi.Paks, name)
		dst := filepath.Join(gi.Backup, name)

		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				res.Skipped = append(res.Skipped, name+"（原位置不存在该文件）")
				continue
			}
			res.OK = false
			res.Errors = append(res.Errors, "无法读取 "+name+"："+err.Error())
			continue
		}

		if _, err := os.Stat(dst); err == nil {
			if cs.decide(name) == ConflictSkip {
				res.Skipped = append(res.Skipped, name+"（备份夹已存在同名文件，已跳过）")
				continue
			}
		}

		n, err := moveFile(src, dst)
		if err != nil {
			res.OK = false
			res.Errors = append(res.Errors, moveErrText("备份", name, err))
			continue
		}
		res.Moved = append(res.Moved, name)
		res.Bytes += n
		Logf("已备份 %s（%d 字节）", name, n)
	}
	return res, nil
}

// Restore 把目标文件从备份文件夹「剪切」回 Paks 目录。
func Restore(gi GameInfo, pol ConflictPolicy, ask ConflictFn) (Result, error) {
	res := NewResult("restore")
	if gi.Backup == "" || gi.Paks == "" {
		msg := "尚未确定游戏目录，无法恢复。"
		res.OK = false
		res.Errors = append(res.Errors, msg)
		return res, errors.New(msg)
	}

	if fi, err := os.Stat(gi.Backup); err != nil || !fi.IsDir() {
		msg := "备份文件夹里没有可恢复的文件"
		res.OK = false
		res.Errors = append(res.Errors, msg)
		return res, nil
	}

	anyFound := false
	for _, name := range TargetFiles {
		if _, err := os.Stat(filepath.Join(gi.Backup, name)); err == nil {
			anyFound = true
			break
		}
	}
	if !anyFound {
		msg := "备份文件夹里没有可恢复的文件"
		res.OK = false
		res.Errors = append(res.Errors, msg)
		return res, nil
	}

	cs := &conflictState{policy: pol, ask: ask}
	for _, name := range TargetFiles {
		src := filepath.Join(gi.Backup, name)
		dst := filepath.Join(gi.Paks, name)

		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				res.Skipped = append(res.Skipped, name+"（备份夹里不存在该文件）")
				continue
			}
			res.OK = false
			res.Errors = append(res.Errors, "无法读取 "+name+"："+err.Error())
			continue
		}

		if _, err := os.Stat(dst); err == nil {
			if cs.decide(name) == ConflictSkip {
				res.Skipped = append(res.Skipped, name+"（目标位置已存在同名文件，已跳过）")
				continue
			}
		}

		n, err := moveFile(src, dst)
		if err != nil {
			res.OK = false
			res.Errors = append(res.Errors, moveErrText("恢复", name, err))
			continue
		}
		res.Moved = append(res.Moved, name)
		res.Bytes += n
		Logf("已恢复 %s（%d 字节）", name, n)
	}
	return res, nil
}

// moveFile 优先用 os.Rename 搬移文件；跨卷时退化为「复制 → 校验 → 删除源」。
// 返回搬移的字节数。
func moveFile(src, dst string) (int64, error) {
	si, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	size := si.Size()

	if err := os.Rename(src, dst); err == nil {
		di, verr := os.Stat(dst)
		if verr != nil {
			return 0, verr
		}
		if di.Size() != size {
			return 0, fmt.Errorf("移动后大小不一致（源 %d，目标 %d）", size, di.Size())
		}
		return size, nil
	} else if !isCrossDevice(err) {
		return 0, err
	}

	return copyThenDelete(src, dst, size)
}

// copyThenDelete 用于跨卷场景：复制到临时文件 → 校验大小 → 替换目标 → 删除源。
func copyThenDelete(src, dst string, size int64) (int64, error) {
	tmp := dst + ".tmp"
	if err := copyFile(src, tmp); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}

	ti, err := os.Stat(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if ti.Size() != size {
		_ = os.Remove(tmp)
		return 0, fmt.Errorf("复制后大小不一致（源 %d，目标 %d）", size, ti.Size())
	}

	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if err := os.Remove(src); err != nil {
		return 0, err
	}
	return size, nil
}

// copyFile 把 src 完整复制到 dst。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()

	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// isCrossDevice 判断错误是否为跨卷（不同磁盘）错误。
func isCrossDevice(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EXDEV) {
		return true
	}
	s := strings.ToLower(err.Error())
	keywords := []string{
		"not same device",
		"different disk",
		"different drive",
		"cannot move the file to a different",
	}
	for _, k := range keywords {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// isFileBusy 判断错误是否为「文件正被其它程序占用」（共享冲突 / 锁定冲突）。
func isFileBusy(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return true
	}
	// 兜底：不同语言的系统错误文案差异较大，按关键字再判一次。
	s := strings.ToLower(err.Error())
	for _, k := range []string{
		"being used by another process",
		"另一个程序正在使用",
		"正由另一进程使用",
		"被另一个进程使用",
	} {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// moveErrText 生成移动失败时给用户看的中文说明。
func moveErrText(action, name string, err error) string {
	if isFileBusy(err) {
		return action + " " + name + " 失败：该文件正被其它程序占用，请关闭正在使用它的程序（例如游戏或 WeGame）后重试。"
	}
	return action + " " + name + " 失败：" + err.Error()
}
