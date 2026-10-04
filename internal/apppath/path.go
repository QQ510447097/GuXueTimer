// Package apppath 负责确定应用的本地数据目录。
//
// 之所以单独成一个包：internal/store 保持只依赖标准库和 SQLite 驱动，
// 可以只读地跑单元测试；只有这里需要碰 Wails 的运行时。
package apppath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// DirName 是应用数据目录名。发布后不要修改，否则会丢历史记录。
const DirName = "GuXueTimer"

// legacyDirName 是改名前的数据目录名（旧名是 GoXueTimer）。
//
// 只在 EnsureDir 里做一次性搬迁，搬完就与它无关了。
const legacyDirName = "GoXueTimer"

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
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, DirName), nil
}

// EnsureDir 返回数据目录并确保它存在；顺带处理一次从旧目录名的搬迁。
//
// 任何要往数据目录写东西的地方都必须走这里（而不是自己 MkdirAll），
// 否则会在新目录里先建出文件，让搬迁逻辑判定"新目录已有数据"而放弃搬迁。
func EnsureDir() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	legacy, legacyErr := legacyDataDir()
	if legacyErr == nil {
		if moved, mErr := migrateLegacyDir(dir, legacy); mErr != nil {
			// 搬迁失败不该挡住启动：新目录照常用，用户会看到历史记录"消失"，
			// 但数据仍在旧目录里，可以人工救回。
			_, _ = fmt.Fprintf(os.Stderr, "apppath: 迁移旧数据目录失败: %v\n", mErr)
		} else if moved {
			_, _ = fmt.Fprintf(os.Stderr, "apppath: 已把数据目录从 %s 迁移到 %s\n", legacy, dir)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// DBPath 返回 SQLite 文件的绝对路径，并确保其父目录已创建。
func DBPath() (string, error) {
	dir, err := EnsureDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dbFileName), nil
}

// baseDir 返回数据目录的父级（Windows 上就是 %LOCALAPPDATA%）。
func baseDir() (string, error) {
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
	abs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func legacyDataDir() (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, legacyDirName), nil
}

// migrateLegacyDir 把旧目录里的内容搬进新目录，返回是否真的搬了东西。
//
// 刻意不用 os.Rename 整体移动目录：新目录可能已经被上一次启动创建成空目录了，
// 那样 Rename 会失败。逐项搬迁可以容忍这种情况，也可以被测试直接驱动。
//
// 判定"要不要搬"的规则是"新目录里没有 timer.db"，而不是"新目录不存在"——
// 只有空目录时才搬，已经有库就绝不覆盖。
func migrateLegacyDir(newDir, oldDir string) (bool, error) {
	if newDir == "" || oldDir == "" || newDir == oldDir {
		return false, nil
	}
	if _, err := os.Stat(oldDir); err != nil {
		return false, nil // 没有旧目录，正常情况
	}
	if _, err := os.Stat(filepath.Join(newDir, dbFileName)); err == nil {
		return false, nil // 新目录已经有库，不动
	}

	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return false, fmt.Errorf("读取旧数据目录失败: %w", err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		return false, err
	}

	moved := false
	for _, entry := range entries {
		src := filepath.Join(oldDir, entry.Name())
		dst := filepath.Join(newDir, entry.Name())
		if _, err := os.Stat(dst); err == nil {
			continue // 同名文件已存在，宁可留下旧的也不覆盖
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, fmt.Errorf("迁移 %s 失败: %w", entry.Name(), err)
		}
		moved = true
	}

	// 旧目录空了就删掉：留一个空壳会让人以为数据还在那。
	if rest, err := os.ReadDir(oldDir); err == nil && len(rest) == 0 {
		_ = os.Remove(oldDir)
	}
	return moved, nil
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
