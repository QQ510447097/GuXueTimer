"""生成应用图标与托盘图标，两者保持同一套视觉语言。

产物：
    build/appicon.png    1024x1024，wails3 generate icons 的输入（进而产出
                         build/windows/icon.ico 与 darwin/icons.icns）
    assets/tray.png      32x32，托盘图标（//go:embed 进二进制）

改外观就改这里的参数再跑一次：

    python tools/gen_icons.py

依赖只有 Pillow（gradient 也没用 numpy，避免多一个依赖）。
"""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parent.parent

# 蓝白配色，和界面里的令牌保持同一组值。
ACCENT_TOP = (91, 155, 255)      # #5B9BFF
ACCENT_BOTTOM = (34, 211, 238)   # #22D3EE
GLYPH = (255, 255, 255, 255)


def diagonal_gradient(size: int, top: tuple, bottom: tuple) -> Image.Image:
    """左上到右下的线性渐变。

    直接逐像素算：1024x1024 用 Pillow 的 putdata 大约几十毫秒，
    比引入 numpy 或者靠缩放一张小图凑要直白得多。
    """
    img = Image.new("RGB", (size, size))
    pixels = []
    denom = 2 * (size - 1) if size > 1 else 1
    for y in range(size):
        for x in range(size):
            t = (x + y) / denom          # 0 → 1，沿对角线
            pixels.append((
                round(top[0] + (bottom[0] - top[0]) * t),
                round(top[1] + (bottom[1] - top[1]) * t),
                round(top[2] + (bottom[2] - top[2]) * t),
            ))
    img.putdata(pixels)
    return img


def clock_glyph(draw: ImageDraw.ImageDraw, cx: float, cy: float, radius: float,
                stroke: float) -> None:
    """画一个"12 点 3 点"的白色时钟。

    指针刻意取正上 + 正右：缩到 16x16 时斜线会糊成一团，正交方向最清楚。
    """
    # 外圈
    draw.ellipse(
        [cx - radius, cy - radius, cx + radius, cy + radius],
        outline=GLYPH,
        width=round(stroke),
    )

    cap = stroke / 2
    inner = radius - stroke * 0.9   # 指针顶端留出和外圈的空隙
    hands = [
        ((cx, cy), (cx, cy - inner * 0.72)),          # 分针指上
        ((cx, cy), (cx + inner * 0.55, cy)),          # 时针指右
    ]
    for start, end in hands:
        draw.line([start, end], fill=GLYPH, width=round(stroke))
        # Pillow 的 line 没有圆头，两端各补一个圆当圆帽。
        for px, py in (start, end):
            draw.ellipse([px - cap, py - cap, px + cap, py + cap], fill=GLYPH)

    # 中心轴
    r_dot = stroke * 0.62
    draw.ellipse([cx - r_dot, cy - r_dot, cx + r_dot, cy + r_dot], fill=GLYPH)


def app_icon(size: int = 1024) -> Image.Image:
    """圆角方形磁贴 + 时钟。"""
    inset = round(size * 0.0625)          # 四周留透明边距
    tile = size - inset * 2
    radius = round(tile * 0.25)           # squircle 观感

    # 磁贴形状的蒙版
    mask = Image.new("L", (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        [inset, inset, size - inset - 1, size - inset - 1], radius=radius, fill=255
    )

    icon = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    gradient = diagonal_gradient(size, ACCENT_TOP, ACCENT_BOTTOM)
    icon.paste(gradient, (0, 0), mask)

    # 左上角一抹高光，做出玻璃质感
    highlight = Image.new("L", (size, size), 0)
    ImageDraw.Draw(highlight).ellipse(
        [-size * 0.35, -size * 0.55, size * 0.95, size * 0.55], fill=64
    )
    highlight = highlight.filter(ImageFilter.GaussianBlur(size * 0.06))
    highlight = Image.composite(highlight, Image.new("L", (size, size), 0), mask)
    icon.paste(Image.new("RGBA", (size, size), (255, 255, 255, 255)), (0, 0), highlight)

    # 内描边：一条很淡的白边，缩小时能让边缘更利落
    draw = ImageDraw.Draw(icon)
    draw.rounded_rectangle(
        [inset + 2, inset + 2, size - inset - 3, size - inset - 3],
        radius=radius - 2,
        outline=(255, 255, 255, 70),
        width=max(2, round(size * 0.004)),
    )

    clock_glyph(draw, size / 2, size / 2, radius=tile * 0.268, stroke=tile * 0.118)
    return icon


def tray_icon(size: int = 32) -> Image.Image:
    """托盘图标：4 倍超采样后缩到 32，保证下采样到 16px 也干净。

    不用圆角方形磁贴而用实心圆：通知区域里方形会显得比邻近的圆形图标重。
    """
    scale = 4
    canvas = size * scale
    img = Image.new("RGBA", (canvas, canvas), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    c = canvas / 2
    r = c - 2 * scale
    draw.ellipse([c - r, c - r, c + r, c + r], fill=ACCENT_TOP + (255,))
    clock_glyph(draw, c, c, radius=r * 0.62, stroke=r * 0.26)

    return img.resize((size, size), Image.LANCZOS)


def main() -> None:
    app_path = ROOT / "build" / "appicon.png"
    tray_path = ROOT / "assets" / "tray.png"

    app_icon().save(app_path, format="PNG", optimize=True)
    print(f"wrote {app_path} ({app_path.stat().st_size} bytes)")

    tray_path.parent.mkdir(parents=True, exist_ok=True)
    tray_icon().save(tray_path, format="PNG", optimize=True)
    print(f"wrote {tray_path} ({tray_path.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
