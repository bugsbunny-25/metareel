"""Draw outlines into Motion Picture Ratings' empty '/' and '+' glyphs, matched
to the font's own strokes: '+' uses the hyphen's bar thickness and height,
'/' the slant and weight of the 7's diagonal, both with the font's soft corners.

The font (tides' certification font) has blank glyphs for everything but
letters, digits, '-' and '.', so ratings like "U/A 16+" and "R18+" lost their
'/' and '+'. Run once on the original file (needs fonttools):

    python3 tools/cert-font-glyphs.py original.ttf assets/fonts/motion_picture_ratings.ttf
"""
import sys
from fontTools.ttLib import TTFont
from fontTools.pens.ttGlyphPen import TTGlyphPen

src, out = sys.argv[1], sys.argv[2]
f = TTFont(src)
cmap = f.getBestCmap()
gs = f.getGlyphSet()

def area(poly):
    return sum(x0 * y1 - x1 * y0 for (x0, y0), (x1, y1) in zip(poly, poly[1:] + poly[:1])) / 2

def draw(polys, r=36):
    pen = TTGlyphPen(gs)
    for poly in polys:
        if area(poly) > 0:  # TrueType outer contours run clockwise
            poly = poly[::-1]
        n = len(poly)
        def towards(a, b, d):
            (ax, ay), (bx, by) = a, b
            L = ((bx - ax) ** 2 + (by - ay) ** 2) ** 0.5
            t = min(d / L, 0.5)
            return (round(ax + (bx - ax) * t), round(ay + (by - ay) * t))
        start = towards(poly[0], poly[1], r)
        pen.moveTo(start)
        for i in range(1, n + 1):
            p, prev, nxt = poly[i % n], poly[i - 1], poly[(i + 1) % n]
            pen.lineTo(towards(p, prev, r))
            pen.qCurveTo(p, towards(p, nxt, r))
        pen.closePath()
    return pen.glyph()

# '+': two bars as thick as the hyphen (497..780 = 283), centred on its height.
T, ARM, LSB = 283, 860, 93
cy = (497 + 780) / 2
cx = LSB + ARM / 2
h, a = T / 2, ARM / 2
plus = [(cx - a, cy - h), (cx - h, cy - h), (cx - h, cy - a), (cx + h, cy - a), (cx + h, cy - h), (cx + a, cy - h),
        (cx + a, cy + h), (cx + h, cy + h), (cx + h, cy + a), (cx - h, cy + a), (cx - h, cy + h), (cx - a, cy + h)]
plus = [(round(x), round(y)) for x, y in plus]

# '/': baseline to cap height (1434), the 7's diagonal: ~0.44 run per rise, ~440 units across.
CAP, RUN, W, SL = 1434, 0.44 * 1434, 440, 60
slash = [(SL, 0), (SL + W, 0), (round(SL + W + RUN), CAP), (round(SL + RUN), CAP)]

glyf, hmtx = f['glyf'], f['hmtx']
for ch, polys, adv in (('+', [plus], LSB * 2 + ARM), ('/', [slash], round(SL * 2 + W + RUN))):
    name = cmap[ord(ch)]
    g = draw(polys)
    g.recalcBounds(glyf)
    glyf[name] = g
    hmtx[name] = (adv, g.xMin)

f.save(out)
print('saved', out)
