"""Tiny toolkit for the README SVGs: font instancing, metrics, subsetting and
embedding, in the same style as the domainreseller profile assets.

Fonts (JetBrains Mono, Inter, Noto Sans Arabic and Noto Sans SC; all SIL OFL)
are downloaded from the google/fonts repository into .fonts/ on first use and
embedded as small WOFF subsets, so every SVG renders identically everywhere
without external requests.

Requires: pip install fonttools"""
import base64
import io
import os
import urllib.request
from html import escape

from fontTools.ttLib import TTFont
from fontTools.varLib import instancer
from fontTools import subset

HERE = os.path.dirname(os.path.abspath(__file__))
FONTDIR = os.path.join(HERE, '.fonts')
FONT_URLS = {
    'JetBrainsMono.ttf': 'https://github.com/google/fonts/raw/main/ofl/jetbrainsmono/JetBrainsMono%5Bwght%5D.ttf',
    'Inter.ttf': 'https://github.com/google/fonts/raw/main/ofl/inter/Inter%5Bopsz,wght%5D.ttf',
    'NotoSansArabic.ttf': 'https://github.com/google/fonts/raw/main/ofl/notosansarabic/NotoSansArabic%5Bwdth,wght%5D.ttf',
    'NotoSansSC.ttf': 'https://github.com/google/fonts/raw/main/ofl/notosanssc/NotoSansSC%5Bwght%5D.ttf',
}

# class -> (file, axes, generic fallback, css family, fallback class, keep OpenType layout)
# Arabic needs its layout features (joining forms); Latin characters in Arabic
# text fall back to Inter per glyph through the CSS font-family list.
FONTS = {
    'R': ('JetBrainsMono.ttf', {'wght': 400}, 'monospace', 'DR', None, False),
    'B': ('JetBrainsMono.ttf', {'wght': 700}, 'monospace', 'DB', None, False),
    'N': ('Inter.ttf', {'wght': 400, 'opsz': 14}, 'sans-serif', 'DN', None, False),
    'S': ('Inter.ttf', {'wght': 600, 'opsz': 14}, 'sans-serif', 'DS', None, False),
    'X': ('Inter.ttf', {'wght': 800, 'opsz': 32}, 'sans-serif', 'DX', None, False),
    'AN': ('NotoSansArabic.ttf', {'wght': 400, 'wdth': 100}, 'sans-serif', 'DAN', 'N', True),
    'AS': ('NotoSansArabic.ttf', {'wght': 600, 'wdth': 100}, 'sans-serif', 'DAS', 'S', True),
    'AX': ('NotoSansArabic.ttf', {'wght': 800, 'wdth': 100}, 'sans-serif', 'DAX', 'X', True),
    'CN': ('NotoSansSC.ttf', {'wght': 400}, 'sans-serif', 'DCN', 'N', False),
    'CS': ('NotoSansSC.ttf', {'wght': 600}, 'sans-serif', 'DCS', 'S', False),
    'CX': ('NotoSansSC.ttf', {'wght': 800}, 'sans-serif', 'DCX', 'X', False),
}

_static = {}


def _ensure_font(fname):
    path = os.path.join(FONTDIR, fname)
    if not os.path.exists(path):
        os.makedirs(FONTDIR, exist_ok=True)
        print(f'downloading {fname}')
        urllib.request.urlretrieve(FONT_URLS[fname], path)
    return path


def static(cls):
    if cls in _static:
        return _static[cls]
    fname, axes, _, fam, _, _ = FONTS[cls]
    cache = os.path.join(FONTDIR, f'static-{fam}.ttf')
    if not os.path.exists(cache):
        f = TTFont(_ensure_font(fname))
        inst = instancer.instantiateVariableFont(f, axes)
        inst.save(cache)
    font = TTFont(cache)
    _static[cls] = font
    return font


def _own(cls, ch):
    return ord(ch) in static(cls).getBestCmap()


def owner(cls, ch):
    """The class whose font draws ch for text of class cls (primary or fallback)."""
    if _own(cls, ch):
        return cls
    fb = FONTS[cls][4]
    if fb and _own(fb, ch):
        return fb
    return None


def has(cls, ch):
    return owner(cls, ch) is not None


def width(cls, s, size):
    total = 0.0
    for ch in s:
        o = owner(cls, ch)
        if o is None:
            raise ValueError(f'glyph {ch!r} (U+{ord(ch):04X}) missing in {cls}')
        f = static(o)
        g = f.getBestCmap()[ord(ch)]
        total += f['hmtx'][g][0] * size / f['head'].unitsPerEm
    return total


def fit(cls, s, size, maxw, minimum=8.0):
    """Largest font size <= size at which s is at most maxw wide."""
    while size > minimum and width(cls, s, size) > maxw:
        size -= 0.25
    return size


class SVG:
    def __init__(self, w, h, label):
        self.w, self.h, self.label = w, h, label
        self.css = []
        self.defs = []
        self.body = []
        self.chars = {}

    def style(self, css):
        self.css.append(css)

    def deff(self, s):
        self.defs.append(s)

    def add(self, s):
        self.body.append(s)

    def text(self, x, y, s, cls, size, fill, extra='', anchor=None, rtl=False):
        for ch in s:
            o = owner(cls, ch)
            if o is None:
                raise ValueError(f'glyph {ch!r} missing in {cls} ({s!r})')
            self.chars.setdefault(o, set()).add(ch)
            self.chars.setdefault(cls, set())
        if rtl:
            extra += ' direction="rtl" unicode-bidi="embed"'
            if anchor is None:
                anchor = 'end'  # the left edge of right-to-left text sits at x
        a = f' text-anchor="{anchor}"' if anchor else ''
        return f'<text x="{fmt(x)}" y="{fmt(y)}" class="{cls}" font-size="{fmt(size)}" fill="{fill}"{a}{extra}>{escape(s, quote=False)}</text>'

    def font_css(self):
        out = []
        for cls in sorted(self.chars):
            fname, axes, generic, fam, fb, layout = FONTS[cls]
            chars = self.chars[cls]
            if chars:
                buf = io.BytesIO()
                font = TTFont(os.path.join(FONTDIR, f'static-{fam}.ttf'))
                opts = subset.Options()
                opts.flavor = 'woff'
                opts.layout_features = ['*'] if layout else []
                opts.name_IDs = [1, 2]
                opts.notdef_outline = False
                opts.hinting = False
                sub = subset.Subsetter(opts)
                sub.populate(text=''.join(sorted(chars | {' '})))
                sub.subset(font)
                # fixed timestamps keep the output byte-identical between builds
                font['head'].created = font['head'].modified = 0
                font.recalcTimestamp = False
                font.flavor = 'woff'
                font.save(buf)
                b64 = base64.b64encode(buf.getvalue()).decode()
                out.append(f"@font-face{{font-family:'{fam}';src:url(data:font/woff;base64,{b64}) format('woff')}}")
            families = f"'{fam}'"
            if fb and fb in self.chars and self.chars[fb]:
                families += f",'{FONTS[fb][3]}'"
            out.append(f".{cls}{{font-family:{families},{generic}}}")
        return ''.join(out)

    def render(self):
        css = self.font_css() + 'text{white-space:pre}' + ''.join(self.css) + \
            '@media (prefers-reduced-motion:reduce){*{animation:none!important}}'
        lab = escape(self.label)
        return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {self.w} {self.h}" width="{self.w}" height="{self.h}" '
                f'role="img" aria-label="{lab}"><title>{lab}</title><defs><style>{css}</style>{"".join(self.defs)}</defs>'
                + ''.join(self.body) + '</svg>')


def fmt(v):
    if isinstance(v, float):
        s = f'{v:.2f}'.rstrip('0').rstrip('.')
        return s
    return str(v)


def pct(t, total):
    return f'{100 * t / total:.2f}'.rstrip('0').rstrip('.') + '%'


# Palette (matches the domainreseller profile assets)
INK = '#212A4D'
MUTED = '#5E6A85'
LINE = '#DCE3EC'
SOFT = '#F5F7FB'
GREEN = '#1CCC99'
GREEN_D = '#0E9F76'
CYAN = '#34CFF4'
CYAN_D = '#0B8DB5'
PURPLE = '#5138EE'
AMBER = '#B7791F'
AMBER_L = '#FEC352'
RED = '#E0533D'
TERM = '#212A4D'
TERM_BAR = '#1A2142'
TERM_TXT = '#E2F5F8'
TERM_DIM = '#8A96B0'
