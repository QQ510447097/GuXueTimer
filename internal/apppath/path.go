// Package apppath 负责确定应用的本地数据目录。
//
// 之所以单独成一个包：internal/store 保持只依赖标准库和 SQLite 驱动，
// 可以只读地跑单元测试；只有这里需要碰 Wails 的运行时。
package apppath

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// DirName 是应用数据目录名。发布后不要修改，否则会丢历史记录。
const DirName = "GoXueTimer"

const dbFileName = "timer.db"

// DataDir 返回应用数据目录的绝对路径（不保证该目录已存在）。
//
// 解析顺序：
//  1. Wails 的 PathDataHome（Windows 上是 %LOCALAPPDATA%）
//  2. %LOCALAPPDATA%
//  3. $XDG_DATA_HOME
//  4. os.UserConfigDir()
//
// 特意不使用 %APPDATA%（漫游目录）：SQLite 文件放在那里会被漫游配置同步，
// 数据库文件不是配置。也不放在 exe 同目录，Program Files 下没有写权限。
func DataDir() (string, error) {
	base := wailsDataHome()
	if base == "" {
		base = os.Getenv("LOCALAPPDATA")
	}
	if base == "" {
		base = os.Getenv("XDG_DATA_HOME")
	}
	if base == "" {
		if d, err := os.UserConfigDir(); err == nil {
			base = d
		}
	}
	if base == "" {
		return "", errors.New("apppath: 无法确定应用数据目录")
	}
	abs, err := filepath.Abs(filepath.Join(base, DirName))
	if err != nil {
		return "", err
	}
	return abs, nil
}

// DBPath 返回 SQLite 文件的绝对路径，并确保其父目录已创建。
func DBPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, dbFileName), nil
}

// wailsDataHome 包一层 recover。
//
// application.Path 预期在 application.New 之前也能安全调用（它读的是 xdg 全局表），
// 但整个应用的启动不应该赌在这上面，所以这里兜底成空串，让调用方降级。
func wailsDataHome() (p string) {
	defer func() {
		if r := recover(); r != nil {
			p = ""
		}
	}()
	return application.Path(application.PathDataHome)
}
