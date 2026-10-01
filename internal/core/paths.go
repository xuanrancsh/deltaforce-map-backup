package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// GameInfo 描述一次解析得到的游戏目录信息。
type GameInfo struct {
	Root   string // 游戏根目录（包含 PackContent 的那一级）
	Paks   string // Paks 目录，即 <Root>\PackContent\Paks
	Backup string // 备份文件夹，即 <Paks>\<BackupDirName>
}

// scanSubPaths 是自动搜索时探测的相对路径（大小写不敏感，由系统处理）。
var scanSubPaths = []string{
	`WeGameApps\rail_apps`,
	`Program Files\WeGame\rail_apps`,
	`Program Files (x86)\WeGame\rail_apps`,
	`WeGame\rail_apps`,
	`Games\rail_apps`,
	`SteamLibrary\steamapps\common`,
	`Program Files (x86)\Steam\steamapps\common`,
	`Program Files\Steam\steamapps\common`,
	`Games`,
}

// isDir 判断路径是否为已存在的目录。
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// resolveRoot 依据 §3.2 的规则把任意候选目录归一化为游戏根目录。
func resolveRoot(dir string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", fmt.Errorf("路径为空，请选择包含 PackContent 的 DeltaForce 文件夹")
	}
	clean := filepath.Clean(dir)

	// 1. <dir>\PackContent\Paks
	if isDir(filepath.Join(clean, "PackContent", "Paks")) {
		return clean, nil
	}
	// 2. <dir>\DeltaForce\PackContent\Paks
	if isDir(filepath.Join(clean, "DeltaForce", "PackContent", "Paks")) {
		return filepath.Join(clean, "DeltaForce"), nil
	}
	// 3. <dir>\Paks 且 <dir> 的父目录名为 PackContent
	if isDir(filepath.Join(clean, "Paks")) &&
		strings.EqualFold(filepath.Base(filepath.Dir(clean)), "PackContent") {
		return filepath.Clean(filepath.Join(clean, "..", "..")), nil
	}
	return "", fmt.Errorf("该文件夹里没有找到 PackContent\\Paks，请选择包含 PackContent 的 DeltaForce 文件夹")
}

// NewGameInfo 依据游戏根目录与备份夹名称构造 GameInfo。
func NewGameInfo(root, backupName string) GameInfo {
	if strings.TrimSpace(backupName) == "" {
		backupName = BackupDirName
	}
	paks := filepath.Join(root, "PackContent", "Paks")
	return GameInfo{
		Root:   root,
		Paks:   paks,
		Backup: filepath.Join(paks, backupName),
	}
}

// NormalizeDir 使用默认备份夹名称归一化目录。
func NormalizeDir(dir string) (GameInfo, error) {
	return NormalizeDirWithBackup(dir, BackupDirName)
}

// NormalizeDirWithBackup 使用指定备份夹名称归一化目录。
func NormalizeDirWithBackup(dir, backupName string) (GameInfo, error) {
	root, err := resolveRoot(dir)
	if err != nil {
		return GameInfo{}, err
	}
	return NewGameInfo(root, backupName), nil
}

// AutoFind 依次尝试「配置 → 注册表 → 常见路径扫描」来定位游戏根目录。
// 返回 (游戏根目录, 来源说明)。来源取值为 config / registry / scan。
func AutoFind() (string, string, error) {
	// 1. 已保存配置
	if cfg, err := LoadConfig(); err == nil && strings.TrimSpace(cfg.GamePath) != "" {
		if root, rerr := resolveRoot(cfg.GamePath); rerr == nil {
			return root, "config", nil
		}
	}

	// 2. 注册表
	if root := findFromRegistry(); root != "" {
		return root, "registry", nil
	}

	// 3. 常见路径扫描
	if root := findFromScan(); root != "" {
		return root, "scan", nil
	}

	return "", "", fmt.Errorf("未能自动找到游戏目录，请点击『手动指定』选择 DeltaForce 文件夹")
}

// findDeltaForceIn 在给定父目录下查找名字以 DeltaForce 开头的子目录，并归一化校验。
func findDeltaForceIn(parent string) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(e.Name()), "deltaforce") {
			continue
		}
		full := filepath.Join(parent, e.Name())
		if root, err := resolveRoot(full); err == nil {
			return root
		}
	}
	return ""
}

// localFixedDrives 返回本机所有「本地固定磁盘」根路径（例如 C:\）。
// 避免探测软驱 / 光驱 / 已断开的网络驱动器导致长时间阻塞。
func localFixedDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(uint32(1)<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		if windows.GetDriveType(windows.StringToUTF16Ptr(root)) == windows.DRIVE_FIXED {
			out = append(out, root)
		}
	}
	return out
}

// findFromScan 扫描本地固定磁盘下的常见目录。总耗时上限 20 秒。
func findFromScan() string {
	deadline := time.Now().Add(20 * time.Second)

	drives := localFixedDrives()
	if len(drives) == 0 {
		// 极端兜底：枚举失败时退回原来的 A–Z 探测。
		for c := 'A'; c <= 'Z'; c++ {
			drives = append(drives, string(c)+":\\")
		}
	}

	for _, drive := range drives {
		if time.Now().After(deadline) {
			return ""
		}
		if !isDir(drive) {
			continue
		}
		for _, rel := range scanSubPaths {
			if time.Now().After(deadline) {
				return ""
			}
			parent := filepath.Join(drive, rel)
			if !isDir(parent) {
				continue
			}
			if root := findDeltaForceIn(parent); root != "" {
				return root
			}
		}
	}
	return ""
}

// findFromRegistry 从 WeGame 相关注册表项中寻找游戏目录。
func findFromRegistry() string {
	type keyRef struct {
		root registry.Key
		path string
	}
	keys := []keyRef{
		{registry.CURRENT_USER, `Software\Tencent\WeGame`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Tencent\WeGame`},
		{registry.LOCAL_MACHINE, `SOFTWARE\Tencent\WeGame`},
	}
	for _, k := range keys {
		if root := scanRegistryKey(k.root, k.path); root != "" {
			return root
		}
	}
	return ""
}

// scanRegistryKey 枚举某个注册表项下所有字符串值，尝试从中推导安装目录。
func scanRegistryKey(root registry.Key, path string) string {
	key, err := registry.OpenKey(root, path, registry.READ)
	if err != nil {
		return ""
	}
	defer key.Close()

	names, err := key.ReadValueNames(-1)
	if err != nil {
		return ""
	}
	for _, name := range names {
		val, _, err := key.GetStringValue(name)
		if err != nil {
			continue
		}
		base := registryInstallBase(val)
		if base == "" {
			continue
		}
		if gameRoot := findDeltaForceIn(base); gameRoot != "" {
			return gameRoot
		}
	}
	return ""
}

// registryInstallBase 判断注册表字符串值是否像安装目录：
// 含 rail_apps 则直接使用；含 WeGameApps 则拼接 rail_apps；否则返回空串。
func registryInstallBase(val string) string {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "rail_apps"):
		return trimmed
	case strings.Contains(lower, "wegameapps"):
		return filepath.Join(trimmed, "rail_apps")
	default:
		return ""
	}
}
