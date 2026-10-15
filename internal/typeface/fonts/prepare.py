"""Generate bundled web fonts and the fixed-width counter (fonttools + brotli)."""
from pathlib import Path
from fontTools.ttLib import TTFont
from fontTools.pens.ttGlyphPen import TTGlyphPen
from fontTools.pens.transformPen import TransformPen

root = Path(__file__).parent


def rename(font, family, style):
    values = {1: family, 2: style, 3: family + " " + style, 4: family + " " + style,
              6: family.replace(" ", "") + "-" + style, 16: family, 17: style}
    for record in list(font["name"].names):
        if record.nameID in values:
            font["name"].setName(values[record.nameID], record.nameID,
                                 record.platformID, record.platEncID, record.langID)


for weight, style in [("medium", "Regular"), ("bold", "Bold")]:
    font = TTFont(root / f"gmarket{weight}.ttf")
    # The original TTF keeps its name. Converted web copies have a new family
    # name because the original license reserves the Gmarket Sans name.
    rename(font, "Gosu UI", style)
    font.flavor = "woff2"
    font.save(root / f"gmarket{weight}.woff2")
    if weight == "medium":
        license = next(r.toUnicode() for r in font["name"].names if r.nameID == 13)
        (root / "gmarket-license.txt").write_text(license, encoding="utf-8")

font = TTFont(root / "chakrabold.ttf")
cmap = font.getBestCmap()
names = [cmap[ord(c)] for c in "0123456789"]
advance = max(font["hmtx"][name][0] for name in names)
glyphs = font.getGlyphSet()
for name in names:
    width, bearing = font["hmtx"][name]
    shift = (advance - width) // 2
    pen = TTGlyphPen(glyphs)
    glyphs[name].draw(TransformPen(pen, (1, 0, 0, 1, shift, 0)))
    font["glyf"][name] = pen.glyph()
    font["hmtx"][name] = (advance, bearing + shift)
rename(font, "Gosu Counter", "Bold")
font.save(root / "counter.ttf")
font.flavor = "woff2"
font.save(root / "counter.woff2")
