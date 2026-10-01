package core

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// maxRecentLogs 是内存里保留的最近日志条数（供 GUI 显示）。
const maxRecentLogs = 200

var (
	logMu      sync.Mutex
	logFile    *os.File
	logger     *log.Logger
	recentLogs []string
)

// LogDir 返回日志目录。
// 若设置了 DFMAP_CONFIG，则日志放在该配置文件同目录下的 logs 子目录；
// 否则放在 %APPDATA%\DeltaForceMapBackup\logs；再取不到则放到 exe 同目录。
func LogDir() string {
	if p := os.Getenv("DFMAP_CONFIG"); p != "" {
		return filepath.Join(filepath.Dir(p), "logs")
	}
	if appdata := os.Getenv("APPDATA"); appdata != "" {
		return filepath.Join(appdata, "DeltaForceMapBackup", "logs")
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exe), "logs")
	}
	return "logs"
}

// InitLog 初始化日志文件，文件名形如 app-YYYYMMDD.log。
// 初始化失败时不会 panic，调用方可忽略返回的错误。
func InitLog() error {
	logMu.Lock()
	defer logMu.Unlock()

	dir := LogDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	name := "app-" + time.Now().Format("20060102") + ".log"
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	logFile = f
	logger = log.New(f, "", log.LstdFlags)
	return nil
}

// Logf 写一条带时间戳的日志，同时保留在内存中供界面显示。
func Logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)

	logMu.Lock()
	if logger != nil {
		logger.Println(msg)
	}
	recentLogs = append(recentLogs, time.Now().Format("15:04:05")+" "+msg)
	if len(recentLogs) > maxRecentLogs {
		recentLogs = recentLogs[len(recentLogs)-maxRecentLogs:]
	}
	logMu.Unlock()

	// 命令行 / 调试信息统一走 stderr，stdout 只保留 JSON 结果。
	fmt.Fprintln(os.Stderr, "[dfmap] "+msg)
}

// RecentLogs 返回内存中最近的日志副本。
func RecentLogs() []string {
	logMu.Lock()
	defer logMu.Unlock()
	out := make([]string, len(recentLogs))
	copy(out, recentLogs)
	return out
}
