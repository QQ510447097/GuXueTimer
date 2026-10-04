"""生成托盘图标 assets/tray.png。

Windows 通知区域需要 16x16 或 32x32：这里按 4 倍超采样绘制再缩到 32x32，
保证下采样到 16x16 时线条依然干净。

配色刻意用"实心色块 + 白色指针"而不是"描边 + 透明内部"：后者在浅色任务栏上
会和背景糊成一片，指针（白色）也会消失。改图标就改这里的参数再跑一次：

    python tools/gen_tray_icon.py
"""

from pathlib import Path

from PIL import Image, ImageDraw

SCALE = 4
SIZE = 32
CANVAS = SIZE * SCALE

FACE = (91, 157, 255, 255)   # 亮蓝底色，浅色/深色任务栏上都有对比度
HAND = (255, 255, 255, 255)

OUT = Path(__file__).resolve().parent.parent / "assets" / "tray.png"


def draw_icon() -> Image.Image:
    img = Image.new("RGBA", (CANVAS, CANVAS), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img)

    c = CANVAS / 2
    # 留 1px（缩到 32x32 后）的透明边距，避免图标贴边被裁。
    radius = c - 2 * SCALE
    draw.ellipse([c - radius, c - radius, c + radius, c + radius], fill=FACE)

    # 指针：长针朝上、短针朝右，构成一眼可辨的"钟"。
    hand_w = int(2.8 * SCALE)
    draw.line([(c, c), (c, c - 0.52 * radius)], fill=HAND, width=hand_w)
    draw.line([(c, c), (c + 0.40 * radius, c)], fill=HAND, width=hand_w)

    r_dot = int(0.11 * radius)
    draw.ellipse([c - r_dot, c - r_dot, c + r_dot, c + r_dot], fill=HAND)

    return img.resize((SIZE, SIZE), Image.LANCZOS)


def main() -> None:
    icon = draw_icon()
    OUT.parent.mkdir(parents=True, exist_ok=True)
    icon.save(OUT, format="PNG", optimize=True)
    print(f"wrote {OUT} ({OUT.stat().st_size} bytes)")


if __name__ == "__main__":
    main()
