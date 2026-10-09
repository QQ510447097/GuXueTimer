package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"goxuetimer/internal/settings"
)

// writePNG 在目录里造一张纯色 PNG，返回路径。
func writePNG(t *testing.T, path string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 30, G: 90, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("编码 PNG 失败: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("写 PNG 失败: %v", err)
	}
	return path
}

// 正常图片：复制进数据目录，用固定名 + 原扩展名。
func TestImportImageFile_AcceptsPNG(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, filepath.Join(t.TempDir(), "我的壁纸.png"), 320, 200)

	name, err := importImageFile(dir, src)
	if err != nil {
		t.Fatalf("importImageFile() 失败: %v", err)
	}
	if name != "background.png" {
		t.Errorf("落地文件名 = %q, 期望 background.png", name)
	}

	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("读回落地文件失败: %v", err)
	}
	want, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("读源文件失败: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Error("复制出来的内容与源文件不一致")
	}
}

// 不是图片：必须被拒，而且要给出能看懂的原因。
func TestImportImageFile_RejectsNonImage(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(t.TempDir(), "假图片.png")
	if err := os.WriteFile(fake, []byte("这不是图片，只是改了个扩展名"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	_, err := importImageFile(dir, fake)
	if err == nil {
		t.Fatal("期望报错，实际通过了")
	}
	if !strings.Contains(err.Error(), "不是能识别的图片") {
		t.Errorf("错误信息不友好: %v", err)
	}
	// 被拒时不应该在数据目录里留下任何东西。
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("被拒之后数据目录里多了 %d 个文件", len(entries))
	}
}

// 太小的图：能解码，但拉满卡片会糊，按"选错文件"拦下。
func TestImportImageFile_RejectsTooSmall(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, filepath.Join(t.TempDir(), "icon.png"), 16, 16)

	_, err := importImageFile(dir, src)
	if err == nil {
		t.Fatal("期望报错，实际通过了")
	}
	if !strings.Contains(err.Error(), "太小") {
		t.Errorf("错误信息应说明太小: %v", err)
	}
}

// 扩展名不可信：内容是 PNG 但文件名是 .bmp 时，按解码出的格式定扩展名，
// 否则浏览器拿不到正确的类型、图显示不出来。
func TestImportImageFile_UsesDecodedFormat(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, filepath.Join(t.TempDir(), "其实是png.bmp"), 128, 128)

	name, err := importImageFile(dir, src)
	if err != nil {
		t.Fatalf("importImageFile() 失败: %v", err)
	}
	if name != "background.png" {
		t.Errorf("落地文件名 = %q, 期望按解码格式取 background.png", name)
	}
}

// 换图时旧文件要清掉：换扩展名之后旧名字就没人记得了，不按前缀清会留垃圾。
func TestImportImageFile_ReplacesOldFile(t *testing.T) {
	dir := t.TempDir()
	pngSrc := writePNG(t, filepath.Join(t.TempDir(), "a.png"), 128, 128)
	if _, err := importImageFile(dir, pngSrc); err != nil {
		t.Fatalf("第一次导入失败: %v", err)
	}

	// 手工造一个"上一张是 jpg"的状态：扩展名不同，名字也不同。
	if err := os.WriteFile(filepath.Join(dir, "background.jpg"), []byte("old"), 0o644); err != nil {
		t.Fatalf("造旧文件失败: %v", err)
	}
	if _, err := importImageFile(dir, pngSrc); err != nil {
		t.Fatalf("第二次导入失败: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录失败: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "background.png" {
		t.Errorf("导入后目录里应当只剩 background.png, 实际 %v", names)
	}
}

// ---------------------------------------------------------------- 资源中间件

// 中间件的安全边界：只有"设置里记着的那一个文件名"能被取到。
func TestUserAssetMiddleware(t *testing.T) {
	h := newHarness(t)

	// 数据目录里放两个文件：一个是当前背景图，另一个是"不该被访问到"的敏感文件。
	background := writePNG(t, filepath.Join(h.dataDir, "background.png"), 128, 128)
	secret := filepath.Join(h.dataDir, "timer.db")
	if err := os.WriteFile(secret, []byte("PRIVATE"), 0o644); err != nil {
		t.Fatalf("造敏感文件失败: %v", err)
	}
	// 顺便放一个同目录下的其它图片，用来验证"名字不对也不给"。
	writePNG(t, filepath.Join(h.dataDir, "other.png"), 128, 128)

	setBackground := func(name string) {
		next := h.b.Settings()
		next.BackgroundImage = name
		if err := h.b.updateSettings(next); err != nil {
			t.Fatalf("updateSettings() 失败: %v", err)
		}
	}
	setBackground("background.png")

	handler := userAssetMiddleware(h.b)
	var nextCalled int
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled++
		_, _ = w.Write([]byte("FRONTEND"))
	})

	cases := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{"命中当前背景图", "/user/background.png", http.StatusOK, "PNG"},
		{"同目录的其它图片也不给", "/user/other.png", http.StatusNotFound, ""},
		{"不许直接要数据库", "/user/timer.db", http.StatusNotFound, ""},
		{"不许路径穿越", "/user/../timer.db", http.StatusNotFound, ""},
		{"前缀后面为空", "/user/", http.StatusNotFound, ""},
		{"非 /user/ 前缀原样转交", "/assets/index.js", http.StatusOK, "FRONTEND"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			handler(next).ServeHTTP(rec, req)

			if rec.Code != c.wantStatus {
				t.Fatalf("状态码 = %d, 期望 %d（body=%q）", rec.Code, c.wantStatus, rec.Body.String())
			}
			if c.wantBody == "PNG" {
				// 内容必须和源文件逐字节一致，说明真发的是那个文件。
				want, err := os.ReadFile(background)
				if err != nil {
					t.Fatalf("读背景图失败: %v", err)
				}
				if !bytes.Equal(rec.Body.Bytes(), want) {
					t.Error("返回的内容与背景图文件不一致")
				}
				return
			}
			if c.wantBody != "" && !strings.Contains(rec.Body.String(), c.wantBody) {
				t.Errorf("body = %q, 期望包含 %q", rec.Body.String(), c.wantBody)
			}
		})
	}

	t.Run("没设置背景图时一律 404", func(t *testing.T) {
		setBackground("")
		rec := httptest.NewRecorder()
		handler(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/user/background.png", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("状态码 = %d, 期望 404", rec.Code)
		}
	})

	t.Run("设置里记着但文件不在时 404", func(t *testing.T) {
		setBackground("background.png")
		if err := os.Remove(background); err != nil {
			t.Fatalf("删背景图失败: %v", err)
		}
		rec := httptest.NewRecorder()
		handler(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/user/background.png", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("状态码 = %d, 期望 404", rec.Code)
		}
	})

	// 这条要在所有 /user/ 用例之后断言，否则计数没有意义。
	if nextCalled != 1 {
		t.Errorf("非 /user/ 请求转交次数 = %d, 期望 1", nextCalled)
	}
}

// ---------------------------------------------------------------- 设置的保留

// 背景图不在 SettingsDTO 里（由个人化服务掌管），保存设置绝不能把它清掉。
// 否则用户改一下主题，背景图就没了。
func TestSettingsService_PreservesBackgroundImage(t *testing.T) {
	h := newHarness(t)

	next := h.b.Settings()
	next.BackgroundImage = "background.png"
	next.CardMode = settings.CardModeImage
	if err := h.b.updateSettings(next); err != nil {
		t.Fatalf("updateSettings() 失败: %v", err)
	}

	// 界面回传的 DTO 里没有背景图字段（它本来就不该有）。
	saved, err := h.settings.Save(SettingsDTO{
		MinimizeToTray: true,
		Theme:          settings.ThemeDark,
		CardMode:       settings.CardModeImage,
	})
	if err != nil {
		t.Fatalf("Save() 失败: %v", err)
	}
	if saved.Theme != settings.ThemeDark {
		t.Errorf("主题没保存: %q", saved.Theme)
	}

	got := h.b.Settings()
	if got.BackgroundImage != "background.png" {
		t.Errorf("保存设置后背景图丢了: %q", got.BackgroundImage)
	}
	if got.CardMode != settings.CardModeImage {
		t.Errorf("保存设置后卡片模式被改了: %q", got.CardMode)
	}
}

// 背景图状态要带上版本号，否则换图之后浏览器一直用缓存里的上一张。
func TestBackgroundState_CarriesVersion(t *testing.T) {
	h := newHarness(t)

	if state := h.b.backgroundState(); state.Image != "" || state.URL != "" {
		t.Errorf("没设置背景图时应当是空状态, 实际 %+v", state)
	}

	writePNG(t, filepath.Join(h.dataDir, "background.png"), 128, 128)
	next := h.b.Settings()
	next.BackgroundImage = "background.png"
	if err := h.b.updateSettings(next); err != nil {
		t.Fatalf("updateSettings() 失败: %v", err)
	}

	state := h.b.backgroundState()
	if state.Image != "background.png" {
		t.Errorf("Image = %q", state.Image)
	}
	if !strings.HasPrefix(state.URL, "/user/background.png?v=") {
		t.Errorf("URL 应当带版本号, 实际 %q", state.URL)
	}
}
