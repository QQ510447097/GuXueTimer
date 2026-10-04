package apppath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读文件失败: %v", err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// 旧目录有库、新目录还不存在：整体搬过去，并把空掉的旧目录删掉。
func TestMigrateLegacyDir_MovesContents(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, legacyDirName)
	newDir := filepath.Join(root, DirName)

	writeFile(t, filepath.Join(oldDir, dbFileName), "old-db")
	writeFile(t, filepath.Join(oldDir, "app.log"), "old-log")

	moved, err := migrateLegacyDir(newDir, oldDir)
	if err != nil {
		t.Fatalf("migrateLegacyDir() 失败: %v", err)
	}
	if !moved {
		t.Fatal("moved = false, 期望 true")
	}
	if got := readFile(t, filepath.Join(newDir, dbFileName)); got != "old-db" {
		t.Errorf("新目录的库 = %q", got)
	}
	if !exists(filepath.Join(newDir, "app.log")) {
		t.Error("app.log 没有被搬过去")
	}
	if exists(oldDir) {
		t.Error("旧目录空了却没有删掉，会让人以为数据还在旧地方")
	}
}

// 关键场景：新目录已被上一次启动创建成空目录，仍然要搬。
// 如果判定规则写成"新目录不存在才搬"，这里就会静默丢历史数据。
func TestMigrateLegacyDir_EmptyNewDirIsMigrated(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, legacyDirName)
	newDir := filepath.Join(root, DirName)

	writeFile(t, filepath.Join(oldDir, dbFileName), "old-db")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatalf("建新目录失败: %v", err)
	}

	moved, err := migrateLegacyDir(newDir, oldDir)
	if err != nil {
		t.Fatalf("migrateLegacyDir() 失败: %v", err)
	}
	if !moved {
		t.Fatal("空的新目录应当被识别为可搬迁")
	}
	if got := readFile(t, filepath.Join(newDir, dbFileName)); got != "old-db" {
		t.Errorf("新目录的库 = %q", got)
	}
}

// 新目录已经有库：绝不动它，也不动旧目录。
func TestMigrateLegacyDir_NewDirWithDBIsUntouched(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, legacyDirName)
	newDir := filepath.Join(root, DirName)

	writeFile(t, filepath.Join(oldDir, dbFileName), "old-db")
	writeFile(t, filepath.Join(newDir, dbFileName), "new-db")

	moved, err := migrateLegacyDir(newDir, oldDir)
	if err != nil {
		t.Fatalf("migrateLegacyDir() 失败: %v", err)
	}
	if moved {
		t.Error("新目录已有库，不应发生搬迁")
	}
	if got := readFile(t, filepath.Join(newDir, dbFileName)); got != "new-db" {
		t.Errorf("新目录的库被覆盖成了 %q", got)
	}
	if got := readFile(t, filepath.Join(oldDir, dbFileName)); got != "old-db" {
		t.Errorf("旧目录被动过: %q", got)
	}
}

func TestMigrateLegacyDir_NoOldDirIsNoop(t *testing.T) {
	root := t.TempDir()
	moved, err := migrateLegacyDir(filepath.Join(root, DirName), filepath.Join(root, legacyDirName))
	if err != nil {
		t.Fatalf("migrateLegacyDir() 失败: %v", err)
	}
	if moved {
		t.Error("没有旧目录时 moved 应为 false")
	}
	if exists(filepath.Join(root, DirName)) {
		t.Error("没有旧目录时不该新建目录（建目录是 EnsureDir 的职责）")
	}
}

// 同名文件不覆盖：宁可把它留在旧目录，也不要盖掉新目录里的东西。
func TestMigrateLegacyDir_DoesNotOverwriteExistingFile(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, legacyDirName)
	newDir := filepath.Join(root, DirName)

	writeFile(t, filepath.Join(oldDir, "keep.txt"), "old")
	writeFile(t, filepath.Join(oldDir, dbFileName), "old-db")
	writeFile(t, filepath.Join(newDir, "keep.txt"), "new")

	moved, err := migrateLegacyDir(newDir, oldDir)
	if err != nil {
		t.Fatalf("migrateLegacyDir() 失败: %v", err)
	}
	if !moved {
		t.Fatal("库文件应当被搬过去")
	}
	if got := readFile(t, filepath.Join(newDir, "keep.txt")); got != "new" {
		t.Errorf("同名文件被覆盖成了 %q", got)
	}
	if got := readFile(t, filepath.Join(oldDir, "keep.txt")); got != "old" {
		t.Errorf("被跳过的文件不该被删除，实际内容 %q", got)
	}
	// 旧目录还有东西，不能删。
	if !exists(oldDir) {
		t.Error("旧目录仍有未搬迁的文件，不该删除")
	}
}

func TestMigrateLegacyDir_SameDirIsNoop(t *testing.T) {
	dir := t.TempDir()
	moved, err := migrateLegacyDir(dir, dir)
	if err != nil {
		t.Fatalf("migrateLegacyDir() 失败: %v", err)
	}
	if moved {
		t.Error("新旧目录相同时不应搬迁")
	}
}

func TestMigrateLegacyDir_EmptyStrings(t *testing.T) {
	if _, err := migrateLegacyDir("", t.TempDir()); err != nil {
		t.Errorf("空 newDir 应直接返回: %v", err)
	}
	if _, err := migrateLegacyDir(t.TempDir(), ""); err != nil {
		t.Errorf("空 oldDir 应直接返回: %v", err)
	}
}

// 目录名必须是新名字，否则等于没改。
func TestDataDir_UsesNewDirectoryName(t *testing.T) {
	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir() 失败: %v", err)
	}
	if !strings.HasSuffix(dir, DirName) {
		t.Errorf("DataDir() = %q, 期望以 %q 结尾", dir, DirName)
	}
	if strings.HasSuffix(dir, legacyDirName) {
		t.Errorf("DataDir() 仍然是旧目录名: %q", dir)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("DataDir() 不是绝对路径: %q", dir)
	}
}
