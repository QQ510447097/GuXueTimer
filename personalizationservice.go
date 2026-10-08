package main

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // 注册解码器：DecodeConfig 只认已注册的格式
	_ "image/jpeg" //
	_ "image/png"  //
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"goxuetimer/internal/settings"
)

// 背景图在数据目录里的固定文件名。
//
// 用固定名（而不是"原文件名"）有两个好处：覆盖时不必先删旧文件，
// 也不会因为用户选了一个叫 `..\..\x.png` 的文件而在数据目录里乱放东西。
// 扩展名跟着源文件走，浏览器要靠它判断类型。
const backgroundBaseName = "background"

// minBackgroundSide 是背景图的最小边长。
//
// 拦一道不是因为"小图不能用"，而是因为小图拉满整块卡片之后糊得没法看，
// 用户多半是选错了文件。64 是个很低的门槛，正常截图都不会低于它。
const minBackgroundSide = 64

// PersonalizationService 管理计时页装饰卡片里用户自选的那张背景图。
//
// 它是个独立服务而不是塞进 SettingsService：这里的操作（弹文件框、复制文件、
// 校验图片）和"读写设置"是两件事，混在一起会让设置服务多出一堆和 IO 有关的依赖。
type PersonalizationService struct {
	backend *backend
}

func NewPersonalizationService(b *backend) *PersonalizationService {
	return &PersonalizationService{backend: b}
}

// PersonalizationDTO 是背景图的当前状态。
type PersonalizationDTO struct {
	// Image 是数据目录里的文件名；空表示用户还没选。
	Image string `json:"image"`
	// URL 是可直接放进 <img src> 的地址，带 ?v= 版本号用来打破浏览器缓存。
	// 换图之后文件名不变（就叫 background.png），不带版本号的话
	// 界面会一直显示上一张——这个缓存问题一定会遇到。
	URL string `json:"url"`
}

// Background 返回当前背景图状态（前端启动时会调它）。
func (p *PersonalizationService) Background() PersonalizationDTO {
	return p.backend.backgroundState()
}

// PickImage 让用户选一张图，校验后复制进数据目录并记为背景图。
func (p *PersonalizationService) PickImage() (PersonalizationDTO, error) {
	if p.backend.app == nil {
		return PersonalizationDTO{}, errors.New("应用尚未就绪，暂时无法选择图片")
	}

	path, err := p.backend.app.Dialog.OpenFile().
		SetTitle("选择一张背景图").
		AddFilter("图片", "*.png;*.jpg;*.jpeg;*.gif").
		PromptForSingleSelection()
	if err != nil {
		return PersonalizationDTO{}, fmt.Errorf("打开文件选择框失败: %w", err)
	}
	if path == "" {
		// 用户取消：不是错误，把当前状态原样返回。
		return p.backend.backgroundState(), nil
	}

	dir, err := p.backend.dataDir()
	if err != nil {
		return PersonalizationDTO{}, err
	}
	name, err := importImageFile(dir, path)
	if err != nil {
		return PersonalizationDTO{}, err
	}

	next := p.backend.Settings()
	next.BackgroundImage = name
	// 选图本身就意味着要用图片模式，省掉用户再点一次。
	next.CardMode = settings.CardModeImage
	if err := p.backend.updateSettings(next); err != nil {
		return PersonalizationDTO{}, err
	}

	slog.Info("已更新背景图", "image", name, "from", filepath.Base(path))
	return p.backend.backgroundState(), nil
}

// ClearImage 移除背景图，并把卡片模式退回语录（否则会剩一张空白卡片）。
func (p *PersonalizationService) ClearImage() (PersonalizationDTO, error) {
	dir, err := p.backend.dataDir()
	if err != nil {
		return PersonalizationDTO{}, err
	}
	removeBackgroundFiles(dir, "")

	next := p.backend.Settings()
	next.BackgroundImage = ""
	// 图没了就把模式退回留空：留一块空面板比"留空"更像是加载失败。
	if next.CardMode == settings.CardModeImage {
		next.CardMode = settings.CardModeNone
	}
	if err := p.backend.updateSettings(next); err != nil {
		return PersonalizationDTO{}, err
	}
	slog.Info("已移除背景图")
	return p.backend.backgroundState(), nil
}

// importImageFile 校验源文件确实是图片，复制到数据目录，返回落地的文件名。
//
// 单独抽出来是为了能测：PickImage 里只剩"弹文件框"这一步没有测试价值，
// 真正的规则（是不是图片、够不够大、用哪个扩展名、旧文件怎么清）都在这里。
//
// 校验用 image.DecodeConfig 而不是扩展名：扩展名是用户随便写的，
// 而"能不能解出一张图的尺寸"是硬证据。它只读文件头，不会把整张大图读进内存。
func importImageFile(dir, srcPath string) (string, error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("打不开这个文件: %w", err)
	}
	defer func() { _ = src.Close() }()

	cfg, format, err := image.DecodeConfig(src)
	if err != nil {
		return "", errors.New("这个文件不是能识别的图片（支持 PNG / JPEG / GIF）")
	}
	if cfg.Width < minBackgroundSide || cfg.Height < minBackgroundSide {
		return "", fmt.Errorf("图片太小了（%d×%d），至少 %d×%d",
			cfg.Width, cfg.Height, minBackgroundSide, minBackgroundSide)
	}
	// 回到文件开头，接下来要把内容整份复制过去。
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("读取这个文件失败: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(srcPath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif":
	default:
		// 扩展名不认识（或没有扩展名）时按解码出来的格式定，
		// 否则浏览器会因为拿不到正确的类型而显示不出来。
		ext = "." + format
	}

	name := backgroundBaseName + ext
	// 换了扩展名时旧文件要清掉，否则数据目录里会一直留着上一张图。
	removeBackgroundFiles(dir, name)

	if err := copyFile(src, filepath.Join(dir, name)); err != nil {
		return "", fmt.Errorf("保存背景图失败: %w", err)
	}
	return name, nil
}

// removeBackgroundFiles 删掉数据目录里所有 background.* 文件（keep 那个除外）。
//
// 按前缀删而不是只删设置里记着的那一个：换扩展名（png -> jpg）之后，
// 设置里已经是新名字，旧名字从此没人记得，只删"记着的那个"会留下垃圾文件。
func removeBackgroundFiles(dir string, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		slog.Warn("读取数据目录失败", "err", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == keep || !strings.HasPrefix(name, backgroundBaseName+".") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			slog.Warn("删除旧背景图失败", "file", name, "err", err)
		}
	}
}

// copyFile 复制文件并落盘。先写临时文件再改名，避免中途失败留下半个文件。
func copyFile(src *os.File, dst string) error {
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Windows 上 os.Rename 覆盖已存在的文件是允许的（内部走 MoveFileEx）。
	return os.Rename(tmp, dst)
}

// userAssetMiddleware 只接管 /user/ 前缀的请求，其余原样交给前端资源处理器。
//
// 安全边界就一条：**只提供设置里记着的那一个文件名**。请求路径里的任何东西
// 都不参与拼路径——拼了就等于让 URL 决定读哪个文件。所以这里不是"把 URL 映射到
// 文件系统"，而是"比对文件名，命中就发那一个已知的绝对路径"。
func userAssetMiddleware(b *backend) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const prefix = "/user/"
			if !strings.HasPrefix(r.URL.Path, prefix) {
				next.ServeHTTP(w, r)
				return
			}

			wanted := strings.TrimPrefix(r.URL.Path, prefix)
			name := b.Settings().BackgroundImage
			if name == "" || wanted != name {
				http.NotFound(w, r)
				return
			}

			dir, err := b.dataDir()
			if err != nil {
				http.Error(w, "读取数据目录失败", http.StatusInternalServerError)
				return
			}
			// name 在 settings.Normalize 里已经过 filepath.Base，
			// 这里再取一次 Base 拼绝对路径，双保险。
			full := filepath.Join(dir, filepath.Base(name))
			if _, err := os.Stat(full); err != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, full)
		})
	}
}
