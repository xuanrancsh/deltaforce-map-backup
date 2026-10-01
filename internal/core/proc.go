package core

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// gameProcKeywords 命中任一项即视为游戏或反作弊正在运行（忽略大小写）。
var gameProcKeywords = []string{
	"deltaforce",
	"ace-guard",
	"anticheatexpert",
	"sguard",
}

// GameRunning 枚举当前进程，判断游戏或反作弊是否在运行。
// 返回 (是否命中, 命中的进程名列表, 错误)。
func GameRunning() (bool, []string, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, nil, err
	}
	defer windows.CloseHandle(snapshot)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return false, nil, err
	}

	var hits []string
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		lower := strings.ToLower(name)
		for _, kw := range gameProcKeywords {
			if strings.Contains(lower, kw) {
				hits = append(hits, name)
				break
			}
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			break
		}
	}

	if len(hits) > 0 {
		return true, hits, nil
	}
	return false, nil, nil
}
