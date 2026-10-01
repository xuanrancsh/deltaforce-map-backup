package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config 是持久化到磁盘的配置。
type Config struct {
	GamePath               string `json:"game_path"`
	BackupDirName          string `json:"backup_dir_name"`
	SuppressConflictPrompt bool   `json:"suppress_conflict_prompt"`
	LastUpdated            string `json:"last_updated"`
}

// DefaultConfig 返回带默认值的配置。
func DefaultConfig() Config {
	return Config{
		BackupDirName: BackupDirName,
	}
}

// ConfigPath 返回配置文件路径。
// 依次尝试：环境变量 DFMAP_CONFIG → %APPDATA%\DeltaForceMapBackup\config.json → exe 同目录 config.json。
func ConfigPath() string {
	if p := os.Getenv("DFMAP_CONFIG"); p != "" {
		return p
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		return filepath.Join(appdata, "DeltaForceMapBackup", "config.json")
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "config.json")
	}
	return "config.json"
}

// LoadConfig 从默认路径读取配置。文件不存在时返回默认配置且不报错。
func LoadConfig() (Config, error) {
	return LoadConfigFrom(ConfigPath())
}

// LoadConfigFrom 从指定路径读取配置。文件不存在时返回默认配置且不报错。
func LoadConfigFrom(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	if cfg.BackupDirName == "" {
		cfg.BackupDirName = BackupDirName
	}
	return cfg, nil
}

// SaveConfig 把配置写回默认路径。
func SaveConfig(cfg Config) error {
	return SaveConfigTo(ConfigPath(), cfg)
}

// SaveConfigTo 把配置写入指定路径：先写临时文件，再 os.Rename，尽量避免写坏。
func SaveConfigTo(path string, cfg Config) error {
	if cfg.BackupDirName == "" {
		cfg.BackupDirName = BackupDirName
	}
	cfg.LastUpdated = time.Now().Format(time.RFC3339)

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// GameInfo 依据配置里的备份夹名称解析出游戏目录信息。
func (c Config) GameInfo() (GameInfo, error) {
	return NormalizeDirWithBackup(c.GamePath, c.BackupDirName)
}
