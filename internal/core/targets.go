// Package core 实现「三角洲地图文件备份与恢复」的核心业务逻辑：
// 目标文件定义、路径解析、自动搜索、配置持久化、备份 / 恢复、进程检测与日志。
package core

// TargetFiles 是需要在 Paks 目录与备份文件夹之间搬移的固定文件列表。
// 注意文件名必须精确（包含 .pak 后缀）。
var TargetFiles = []string{
	"pak-0-2-pakchunk90-WindowsClient.pak",
	"pak-0-2-pakchunk90optional-WindowsClient.pak",
}

// BackupDirName 是备份文件夹的默认名称（可在配置中修改）。
const BackupDirName = "MapBackup"
