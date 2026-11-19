"""Generate the animated README SVGs into assets/ (English) and assets/<lang>/.

    pip install fonttools
    python scripts/readme-assets/build_assets.py

Edit the texts (strings.py), layout and timings here, never the generated
SVGs: each SVG embeds a font subset that only contains the characters it uses."""
import os
import sys

from svgkit import (SVG, width, fit, fmt, pct, INK, MUTED, LINE, SOFT, GREEN, GREEN_D, CYAN, CYAN_D,
                    PURPLE, AMBER, AMBER_L, RED, TERM, TERM_BAR, TERM_TXT, TERM_DIM)
from strings import S, LANG_ORDER, LANG_PILLS

ROOT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', 'assets')

COMMON_DEFS = (
    f'<linearGradient id="ag" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="{GREEN}"/><stop offset="1" stop-color="{CYAN}"/></linearGradient>'
    '<linearGradient id="hb" x1="0" y1="0" x2=".3" y2="1"><stop offset="0" stop-color="#FFFFFF"/><stop offset=".55" stop-color="#F2FBF8"/><stop offset="1" stop-color="#E2F5F8"/></linearGradient>'
    '<pattern id="dots" width="24" height="24" patternUnits="userSpaceOnUse"><circle cx="2" cy="2" r="1.1" fill="#212A4D" fill-opacity=".07"/></pattern>'
    f'<radialGradient id="gC"><stop offset="0" stop-color="{CYAN}" stop-opacity=".30"/><stop offset="1" stop-color="{CYAN}" stop-opacity="0"/></radialGradient>'
    f'<radialGradient id="gG"><stop offset="0" stop-color="{GREEN}" stop-opacity=".26"/><stop offset="1" stop-color="{GREEN}" stop-opacity="0"/></radialGradient>'
    f'<radialGradient id="gP"><stop offset="0" stop-color="{PURPLE}" stop-opacity=".16"/><stop offset="1" stop-color="{PURPLE}" stop-opacity="0"/></radialGradient>'
)

BASE_CSS = (
    '.fl{animation:fl 7s ease-in-out infinite alternate}@keyframes fl{to{transform:translateY(-12px)}}'
    '.fl2{animation:fl2 9s ease-in-out infinite alternate}@keyframes fl2{to{transform:translate(10px,8px)}}'
    '.pu{animation:pu 1.8s ease-in-out infinite}@keyframes pu{50%{opacity:.25}}'
    '.bl{animation:bl 1s steps(1) infinite}@keyframes bl{50%{opacity:0}}'
    '.ping{transform-box:fill-box;transform-origin:center;animation:ping 2.8s ease-out infinite}'
    '@keyframes ping{from{transform:scale(.6);opacity:.9}to{transform:scale(3.4);opacity:0}}'
    '.orb{animation:orb 5s linear infinite}@keyframes orb{to{stroke-dashoffset:-64}}'
    '.scan{animation:scan 7s ease-in-out infinite}@keyframes scan{0%,100%{stroke-opacity:0}8%{stroke-opacity:1}22%{stroke-opacity:0}}'
    '.bob{animation:bob 4s ease-in-out infinite}@keyframes bob{50%{transform:translateY(-3px)}}'
)


class Lang:
    """Per-language font classes and direction."""

    def __init__(self, code):
        self.code = code
        self.t = S[code]
        self.rtl = code == 'sa'
        m = {'sa': ('AN', 'AS', 'AX', 'AN', 'AS'), 'cn': ('CN', 'CS', 'CX', 'CN', 'CS')}.get(code, ('N', 'S', 'X', 'R', 'B'))
        self.N, self.S, self.X, self.chat, self.chatB = m
        self.dir = ROOT if code == 'en' else os.path.join(ROOT, code)


def write(lang, name, svg):
    os.makedirs(lang.dir, exist_ok=True)
    data = svg.render()
    with open(os.path.join(lang.dir, name), 'w', encoding='utf-8', newline='\n') as f:
        f.write(data)
    return len(data)


def pill(svg, x, y, label, fill, txt='#FFFFFF', size=11, padx=8, h=20, cls='B', rtl=False):
    w = width(cls, label, size) + 2 * padx
    t = svg.text(x + w / 2, y + h / 2 + size * 0.36, label, cls, size, txt, anchor='middle', rtl=rtl)
    return w, f'<rect x="{fmt(x)}" y="{fmt(y)}" width="{fmt(w)}" height="{h}" rx="{h / 2}" fill="{fill}"/>' + t


# --------------------------------------------------------------------------- hero

def hero(lang):
    t = lang.t
    W, H, T = 1200, 560, 16.0
    s = SVG(W, H, t['hero_aria'])
    s.deff(COMMON_DEFS)
    s.style(BASE_CSS)
    s.add(f'<rect x="1" y="1" width="{W - 2}" height="{H - 2}" rx="28" fill="url(#hb)" stroke="{LINE}" stroke-width="2"/>')
    s.add(f'<rect x="1" y="1" width="{W - 2}" height="{H - 2}" rx="28" fill="url(#dots)"/>')
    s.add('<g class="fl"><circle cx="1060" cy="70" r="230" fill="url(#gC)"/></g>')
    s.add('<g class="fl2"><circle cx="170" cy="540" r="220" fill="url(#gG)"/></g>')
    s.add('<g class="fl"><circle cx="560" cy="300" r="160" fill="url(#gP)"/></g>')

    L, MAXW = 56, 524
    rtl = lang.rtl

    def col(y, text, cls, size, fill):
        """Left-column text: left aligned, or right aligned for right-to-left languages."""
        if rtl:
            return s.text(L + MAXW, y, text, cls, size, fill, anchor='start', rtl=True)
        return s.text(L, y, text, cls, size, fill)

    bsz = fit(lang.S, t['badge'], 13, MAXW - 48)
    bw = width(lang.S, t['badge'], bsz) + 48
    bx = L + MAXW - bw if rtl else L
    s.add(f'<rect x="{fmt(bx)}" y="56" width="{fmt(bw)}" height="32" rx="16" fill="#FFFFFF" stroke="#B5F0E1" stroke-width="1.5"/>')
    s.add(f'<circle cx="{fmt(bx + 18)}" cy="72" r="4.5" fill="{GREEN}" class="ping"/><circle cx="{fmt(bx + 18)}" cy="72" r="4.5" fill="{GREEN}"/>')
    s.add(s.text(bx + 32, 76.5, t['badge'], lang.S, bsz, GREEN_D, rtl=rtl))

    size = 50
    while max(width(lang.X, t['t1'], size), width(lang.X, t['t2'], size)) > MAXW:
        size -= 0.5
    s.add(col(152, t['t1'], lang.X, size, INK))
    s.add(col(152 + size * 1.18, t['t2'], lang.X, size, 'url(#ag)'))

    ssz = 17
    while max(width(lang.N, x, ssz) for x in t['sub']) > MAXW:
        ssz -= 0.25
    y = 152 + size * 1.18 + 46
    for line in t['sub']:
        s.add(col(y, line, lang.N, ssz, MUTED))
        y += ssz * 1.55

    colors = [GREEN, PURPLE, CYAN_D, AMBER, RED, GREEN_D]
    csz = 13
    while sum(width(lang.S, c, csz) + 50 for c in t['chips']) > 2 * MAXW + 20 and csz > 10:
        csz -= 0.25
    cx, cy = L, y + 8
    for i, (label, color) in enumerate(zip(t['chips'], colors)):
        cw = width(lang.S, label, csz) + 40
        if cx + cw > L + MAXW:
            cx, cy = L, cy + 44
        s.add(f'<g class="bob" style="animation-delay:{i * 0.35:.2f}s">'
              f'<rect x="{fmt(cx)}" y="{fmt(cy)}" width="{fmt(cw)}" height="32" rx="10" fill="#FFFFFF" stroke="{LINE}" stroke-width="1.5"/>'
              f'<circle cx="{fmt(cx + 17)}" cy="{fmt(cy + 16)}" r="4" fill="{color}"/>'
              + s.text(cx + 28, cy + 20.5, label, lang.S, csz, INK, rtl=rtl) + '</g>')
        cx += cw + 10

    by = cy + 62
    s.add(col(by - 12, t['plug'], lang.S, 13, MUTED))
    cmd = 'claude mcp add dna -- dna mcp serve'
    s.add(f'<rect x="{L}" y="{fmt(by)}" width="{MAXW}" height="52" rx="14" fill="{TERM}"/>')
    s.add(s.text(L + 22, by + 31, '$', 'B', 15, GREEN))
    s.add(s.text(L + 40, by + 31, cmd, 'R', 15, TERM_TXT))
    s.add(f'<rect x="{fmt(L + 44 + width("R", cmd, 15))}" y="{fmt(by + 17)}" width="9" height="18" fill="{GREEN}" class="bl"/>')

    # ---- terminal
    tx, ty, tw, th = 612, 56, 532, 448
    s.add(f'<g class="bob" style="animation-duration:6s">'
          f'<rect x="{tx}" y="{ty + 8}" width="{tw}" height="{th}" rx="18" fill="{INK}" fill-opacity=".12"/>'
          f'<rect x="{tx}" y="{ty}" width="{tw}" height="{th}" rx="18" fill="{TERM}"/>'
          f'<path d="M{tx} {ty + 40}V{ty + 18}a18 18 0 0 1 18-18h{tw - 36}a18 18 0 0 1 18 18v22z" fill="{TERM_BAR}"/>'
          f'<circle cx="{tx + 22}" cy="{ty + 20}" r="6" fill="#FF5F57"/><circle cx="{tx + 42}" cy="{ty + 20}" r="6" fill="#FEBC2E"/><circle cx="{tx + 62}" cy="{ty + 20}" r="6" fill="#28C840"/>'
          + s.text(tx + tw / 2, ty + 24.5, t['term'], lang.chat, 12, TERM_DIM, anchor='middle', rtl=rtl))

    lines = [
        ('agent', t['m1'], 0.4),
        ('cmd', 'dna domain search brandname.com -o json', 1.3),
        ('out', '{"info":{"status":"AVAILABLE","price":11.99}}', 2.9, GREEN),
        ('cmd', 'dna domain register brandname.com --dry-run', 3.7),
        ('out', '{"dryRun":true,"safety":"billable",...}', 5.3, AMBER_L),
        ('agent', t['m2'], 6.2),
        ('you', t['m3'], 7.6),
        ('cmd', 'dna domain register brandname.com --yes', 8.5),
        ('out', '✓ Active · expires 2027-11-19 · 11.99 USD', 10.2, GREEN),
    ]
    x0, y = tx + 24, ty + 76
    for i, ln in enumerate(lines):
        kind, text, start = ln[0], ln[1], ln[2]
        s.style(f'.v{i}{{animation:v{i} {T}s linear infinite}}@keyframes v{i}{{0%,{pct(start, T)}{{opacity:0;transform:translateY(4px)}}'
                f'{pct(start + 0.25, T)},93%{{opacity:1;transform:none}}97%,100%{{opacity:0;transform:none}}}}')
        g = [f'<g class="v{i}">']
        if kind in ('agent', 'you'):
            color = PURPLE if kind == 'agent' else GREEN_D
            pw, ps = pill(s, x0, y - 14, t[kind], color, size=11, cls=lang.chatB, rtl=rtl)
            g.append(ps)
            msz = fit(lang.chat, text, 13, tw - 48 - pw - 10, 10)
            g.append(s.text(x0 + pw + 10, y, text, lang.chat, msz, TERM_TXT, rtl=rtl))
        elif kind == 'cmd':
            cw = width('R', text, 13)
            n = len(text)
            dur = min(1.3, max(0.8, n * 0.03))
            tid = f'c{i}'
            s.deff(f'<clipPath id="{tid}"><rect class="t{i}" x="{fmt(x0 + 16)}" y="{fmt(y - 15)}" width="{fmt(cw + 4)}" height="21"/></clipPath>')
            s.style(f'.t{i}{{animation:t{i} {T}s steps({n},end) infinite}}@keyframes t{i}{{0%,{pct(start + 0.2, T)}{{transform:translateX(-{fmt(cw + 4)}px)}}'
                    f'{pct(start + 0.2 + dur, T)},100%{{transform:none}}}}')
            s.style(f'.k{i}{{animation:k{i} {T}s steps({n},end) infinite}}@keyframes k{i}{{0%,{pct(start + 0.2, T)}{{transform:none;opacity:1}}'
                    f'{pct(start + 0.2 + dur, T)}{{transform:translateX({fmt(cw)}px);opacity:1}}{pct(start + 0.7 + dur, T)},100%{{transform:translateX({fmt(cw)}px);opacity:0}}}}')
            g.append(s.text(x0, y, '$', 'B', 13, GREEN))
            g.append(f'<g clip-path="url(#{tid})">' + s.text(x0 + 16, y, text, 'R', 13, TERM_TXT) + '</g>')
            g.append(f'<rect class="k{i}" x="{fmt(x0 + 17)}" y="{fmt(y - 12)}" width="8" height="15" fill="{GREEN}" fill-opacity=".85"/>')
        elif text.startswith('✓'):
            g.append(s.text(x0 + 15, y + 0.5, '✓', 'S', 15, ln[3]))
            g.append(s.text(x0 + 33, y, text[2:], 'R', 13, ln[3]))
        else:
            g.append(s.text(x0 + 16, y, text, 'R', 13, ln[3]))
        g.append('</g>')
        s.add(''.join(g))
        y += 38
    s.add(f'<path d="M{tx + 1} {ty + th - 34}h{tw - 2}" stroke="#3D4A6B" stroke-width="1"/>')
    s.add(f'<circle cx="{tx + 26}" cy="{ty + th - 17}" r="4" fill="{GREEN}" class="pu"/>')
    s.add(s.text(tx + 38, ty + th - 13, 'ote sandbox   ·   65 MCP tools   ·   exit 0', 'R', 12, TERM_DIM))
    s.add('</g>')
    return write(lang, 'hero.svg', s)


# --------------------------------------------------------------------------- stats

def stats(lang):
    t = lang.t
    W, H = 1200, 160
    s = SVG(W, H, t['stats_aria'])
    s.deff(COMMON_DEFS)
    s.style(BASE_CSS)
    s.style('.rise{animation:rise 1.4s cubic-bezier(.15,.85,.25,1) both}@keyframes rise{from{transform:translateY(26px);opacity:0}}'
            '.bar{transform-box:fill-box;transform-origin:left;animation:bar 1.6s cubic-bezier(.2,.8,.2,1) both}@keyframes bar{from{transform:scaleX(0)}}')
    nums = ['64', '65', '4', '6']
    colors = [GREEN, PURPLE, AMBER, CYAN_D]
    tw, gap, x = 282, 14, 9
    for i, (num, (label, sub), color) in enumerate(zip(nums, t['stats'], colors)):
        d = f'animation-delay:{0.15 + i * 0.18:.2f}s'
        s.add(f'<rect x="{x}" y="8" width="{tw}" height="{H - 16}" rx="22" fill="#FFFFFF" stroke="{LINE}" stroke-width="2"/>')
        s.add(f'<rect x="{x}" y="8" width="{tw}" height="{H - 16}" rx="22" fill="none" stroke="url(#ag)" stroke-width="2" class="scan" style="animation-delay:{i * 1.4:.1f}s"/>')
        s.add(f'<g class="rise" style="{d}">' + s.text(x + 26, 78, num, 'X', 46, 'url(#ag)') + '</g>')
        nw = width('X', num, 46)
        lsz = fit(lang.S, label, 17, tw - 26 - nw - 12 - 18, 11)
        s.add(f'<g class="rise" style="{d}">' + s.text(x + 26 + nw + 12, 62, label, lang.S, lsz, INK, rtl=lang.rtl) + '</g>')
        ssz = fit(lang.N, sub, 13, tw - 52, 9)
        s.add(s.text(x + 26, 112, sub, lang.N, ssz, MUTED, rtl=lang.rtl))
        s.add(f'<rect x="{x + 26}" y="126" width="{tw - 52}" height="5" rx="2.5" fill="{SOFT}"/>')
        s.add(f'<rect x="{x + 26}" y="126" width="{(tw - 52) * (0.55 + 0.12 * i):.0f}" height="5" rx="2.5" fill="{color}" class="bar" style="{d}"/>')
        x += tw + gap
    return write(lang, 'stats.svg', s)


# --------------------------------------------------------------------------- flow

def icon_agent(x, y, c):
    return (f'<g transform="translate({x} {y})"><rect x="0" y="6" width="34" height="26" rx="8" fill="none" stroke="{c}" stroke-width="2.4"/>'
            f'<path d="M17 6V0" stroke="{c}" stroke-width="2.4" stroke-linecap="round"/><circle cx="17" cy="0" r="2.6" fill="{c}" class="pu"/>'
            f'<g class="bl" style="animation-duration:3.2s"><circle cx="11" cy="18" r="2.6" fill="{c}"/><circle cx="23" cy="18" r="2.6" fill="{c}"/></g>'
            f'<path d="M12 25h10" stroke="{c}" stroke-width="2.2" stroke-linecap="round"/></g>')


def icon_shield(x, y, c):
    return (f'<g transform="translate({x} {y})"><path d="M17 0 32 6v11c0 9-6.5 15-15 18C8.5 32 2 26 2 17V6z" fill="none" stroke="{c}" stroke-width="2.4" stroke-linejoin="round"/>'
            f'<path d="M10 17l5 5 9-10" fill="none" stroke="{c}" stroke-width="2.6" stroke-linecap="round" stroke-linejoin="round" class="pu"/></g>')


def icon_server(x, y, c):
    out = [f'<g transform="translate({x} {y})">']
    for i in range(3):
        out.append(f'<rect x="0" y="{i * 12}" width="34" height="9" rx="3" fill="none" stroke="{c}" stroke-width="2.2"/>'
                   f'<circle cx="27" cy="{i * 12 + 4.5}" r="1.9" fill="{c}" class="pu" style="animation-delay:{i * 0.4:.1f}s"/>')
    return ''.join(out) + '</g>'


def icon_globe(x, y, c):
    return (f'<g transform="translate({x} {y})"><circle cx="17" cy="17" r="15" fill="none" stroke="{c}" stroke-width="2.4"/>'
            f'<ellipse cx="17" cy="17" rx="6.5" ry="15" fill="none" stroke="{c}" stroke-width="2"/>'
            f'<path d="M3 12h28M3 22h28" stroke="{c}" stroke-width="2"/></g>')


def flow(lang):
    t = lang.t
    W, H, T = 1200, 300, 2.4
    s = SVG(W, H, t['flow_aria'])
    s.deff(COMMON_DEFS)
    s.style(BASE_CSS)
    meta = [(PURPLE, icon_agent), (GREEN_D, icon_shield), (CYAN_D, icon_server), (AMBER, icon_globe)]
    labels = ['MCP / CLI', 'HTTPS', 'EPP']
    nw, nh, y0 = 212, 168, 66
    gap = (W - 2 * 24 - 4 * nw) / 3
    xs = [24 + i * (nw + gap) for i in range(4)]
    s.add(f'<rect x="1" y="1" width="{W - 2}" height="{H - 2}" rx="26" fill="url(#hb)" stroke="{LINE}" stroke-width="2"/>')
    s.add(f'<rect x="1" y="1" width="{W - 2}" height="{H - 2}" rx="26" fill="url(#dots)"/>')
    for i in range(3):
        xa, xb = xs[i] + nw, xs[i + 1]
        cy = y0 + nh / 2
        s.add(f'<path d="M{fmt(xa + 6)} {fmt(cy)}H{fmt(xb - 6)}" stroke="{LINE}" stroke-width="3"/>')
        s.add(f'<path d="M{fmt(xa + 6)} {fmt(cy)}H{fmt(xb - 6)}" stroke="url(#ag)" stroke-width="3" stroke-dasharray="6 10" class="orb"/>')
        lw = width('S', labels[i], 12) + 20
        mx = (xa + xb) / 2
        s.add(f'<rect x="{fmt(mx - lw / 2)}" y="{fmt(cy - 46)}" width="{fmt(lw)}" height="24" rx="12" fill="#FFFFFF" stroke="{LINE}" stroke-width="1.5"/>'
              + s.text(mx, cy - 29.5, labels[i], 'S', 12, MUTED, anchor='middle'))
        dist = xb - xa - 28
        s.style(f'.q{i}{{animation:q{i} {T}s linear infinite}}@keyframes q{i}{{0%{{transform:translateX(0);opacity:0}}15%,85%{{opacity:1}}100%{{transform:translateX({fmt(dist)}px);opacity:0}}}}'
                f'.r{i}{{animation:r{i} {T}s linear infinite}}@keyframes r{i}{{0%{{transform:translateX(0);opacity:0}}15%,85%{{opacity:1}}100%{{transform:translateX(-{fmt(dist)}px);opacity:0}}}}')
        s.add(f'<circle cx="{fmt(xa + 14)}" cy="{fmt(cy - 7)}" r="5" fill="{GREEN}" class="q{i}" style="animation-delay:{i * 0.3:.1f}s"/>')
        s.add(f'<circle cx="{fmt(xb - 14)}" cy="{fmt(cy + 7)}" r="5" fill="{CYAN}" class="r{i}" style="animation-delay:{0.9 + i * 0.3:.1f}s"/>')
    for i, ((color, icon), title, sub) in enumerate(zip(meta, t['nodes'], t['node_subs'])):
        x = xs[i]
        hl = i == 1
        s.add(f'<rect x="{fmt(x)}" y="{y0}" width="{nw}" height="{nh}" rx="20" fill="#FFFFFF" stroke="{color if hl else LINE}" stroke-width="{2.5 if hl else 2}"/>')
        if hl:
            s.add(f'<rect x="{fmt(x)}" y="{y0}" width="{nw}" height="{nh}" rx="20" fill="none" stroke="url(#ag)" stroke-width="3" class="scan" style="animation-duration:4s"/>')
        s.add(icon(fmt(x + nw / 2 - 17), y0 + 24, color))
        if title in ('dna', 'Domain Name API'):
            tcls = 'B' if title == 'dna' else 'S'
        else:
            tcls = lang.S
        tsz = fit(tcls, title, 18, nw - 20, 12)
        s.add(s.text(x + nw / 2, y0 + 92, title, tcls, tsz, INK, anchor='middle', rtl=lang.rtl and tcls == lang.S))
        for j, line in enumerate(sub):
            lsz = fit(lang.N, line, 13, nw - 20, 9)
            s.add(s.text(x + nw / 2, y0 + 118 + 20 * j, line, lang.N, lsz, MUTED, anchor='middle', rtl=lang.rtl))
    fsz = fit(lang.N, t['flow_foot'], 13, W - 60, 9)
    s.add(s.text(W / 2, H - 22, t['flow_foot'], lang.N, fsz, MUTED, anchor='middle', rtl=lang.rtl))
    return write(lang, 'flow.svg', s)


# --------------------------------------------------------------------------- safety

def safety(lang):
    t = lang.t
    W, H, T = 1200, 300, 6.0
    s = SVG(W, H, t['safety_aria'])
    s.deff(COMMON_DEFS)
    s.style(BASE_CSS)
    s.add(f'<rect x="1" y="1" width="{W - 2}" height="{H - 2}" rx="26" fill="#FFFFFF" stroke="{LINE}" stroke-width="2"/>')
    lanes = [('read', GREEN, False), ('write', CYAN_D, False), ('destructive', AMBER, True), ('billable', RED, True)]
    x_track0, x_gate, x_end = 520, 820, 1040
    y = 52
    s.style(f'.go{{animation:go {T}s linear infinite}}@keyframes go{{0%{{transform:translateX(0);opacity:0}}6%{{opacity:1}}88%{{opacity:1}}100%{{transform:translateX({x_end - x_track0 - 30}px);opacity:0}}}}'
            f'.hold{{animation:hold {T}s cubic-bezier(.4,0,.2,1) infinite}}@keyframes hold{{0%{{transform:translateX(0);opacity:0}}6%{{opacity:1}}34%,62%{{transform:translateX({x_gate - x_track0 - 34}px)}}88%{{opacity:1}}100%{{transform:translateX({x_end - x_track0 - 30}px);opacity:0}}}}'
            f'.arm{{animation:arm {T}s ease-in-out infinite}}@keyframes arm{{0%,54%{{transform:none;opacity:1}}60%,90%{{transform:translateY(-15px);opacity:.18}}100%{{transform:none;opacity:1}}}}'
            f'.ask{{animation:ask {T}s linear infinite}}@keyframes ask{{0%,32%{{opacity:0}}36%,56%{{opacity:1}}60%,100%{{opacity:0}}}}'
            f'.ok{{animation:ok {T}s linear infinite}}@keyframes ok{{0%,56%{{opacity:0}}60%,80%{{opacity:1}}84%,100%{{opacity:0}}}}')
    for i, ((name, color, gated), ex) in enumerate(zip(lanes, t['lanes'])):
        cy = y + 26
        pw, ps = pill(s, 34, cy - 13, name, color, size=13, padx=12, h=26, cls='B')
        s.add(ps)
        ex_x = 34 + pw + 14
        esz = fit(lang.N, ex, 14, x_track0 - 18 - ex_x, 9)
        s.add(s.text(ex_x, cy + 5, ex, lang.N, esz, MUTED, rtl=lang.rtl))
        s.add(f'<path d="M{x_track0} {cy}H{x_end}" stroke="{SOFT}" stroke-width="12" stroke-linecap="round"/>')
        s.add(f'<path d="M{x_track0} {cy}H{x_end}" stroke="{LINE}" stroke-width="2" stroke-dasharray="4 8" class="orb"/>')
        delay = f'animation-delay:{i * 0.25:.2f}s'
        cls = 'hold' if gated else 'go'
        s.add(f'<g class="{cls}" style="{delay}"><rect x="{x_track0 + 6}" y="{cy - 11}" width="22" height="22" rx="7" fill="{color}"/>'
              f'<path d="M{x_track0 + 12} {cy}h10M{x_track0 + 18} {cy - 5}l5 5-5 5" stroke="#FFFFFF" stroke-width="2.2" fill="none" stroke-linecap="round" stroke-linejoin="round"/></g>')
        if gated:
            s.add(f'<g class="arm" style="{delay}"><rect x="{x_gate - 3}" y="{cy - 16}" width="6" height="32" rx="3" fill="{color}"/>'
                  f'<rect x="{x_gate - 9}" y="{cy - 16}" width="18" height="5" rx="2.5" fill="{INK}"/><rect x="{x_gate - 9}" y="{cy + 11}" width="18" height="5" rx="2.5" fill="{INK}"/></g>')
            q = '--yes  /  confirm=true'
            qw = width('R', q, 12) + 20
            s.add(f'<g class="ask" style="{delay}"><rect x="{x_gate - qw / 2:.1f}" y="{cy - 40}" width="{qw:.1f}" height="21" rx="10.5" fill="{INK}"/>'
                  + s.text(x_gate, cy - 25.5, q, 'R', 12, AMBER_L, anchor='middle') + '</g>')
            ok = t['approved']
            ow = width(lang.S, ok, 13) + 22
            s.add(f'<g class="ok" style="{delay}"><rect x="{x_gate - ow / 2:.1f}" y="{cy - 40}" width="{ow:.1f}" height="21" rx="10.5" fill="#E6FAF3" stroke="#B5F0E1"/>'
                  + s.text(x_gate, cy - 25, ok, lang.S, 13, GREEN_D, anchor='middle', rtl=lang.rtl) + '</g>')
        y += 60
    s.add(f'<rect x="{x_end + 12}" y="52" width="130" height="208" rx="18" fill="{SOFT}" stroke="{LINE}" stroke-width="2"/>')
    s.add(icon_server(x_end + 60, 120, CYAN_D))
    s.add(s.text(x_end + 77, 196, 'Domain', 'S', 14, INK, anchor='middle'))
    s.add(s.text(x_end + 77, 214, 'Name API', 'S', 14, INK, anchor='middle'))
    return write(lang, 'safety.svg', s)


# --------------------------------------------------------------------------- mcp

def mcp(lang):
    t = lang.t
    W, H, T = 1200, 300, 12.0
    s = SVG(W, H, t['mcp_aria'])
    s.deff(COMMON_DEFS)
    s.style(BASE_CSS)
    s.add(f'<rect x="1" y="1" width="{W - 2}" height="{H - 2}" rx="26" fill="url(#hb)" stroke="{LINE}" stroke-width="2"/>')
    px, py, pw_, ph = 24, 24, 430, 252
    s.add(f'<rect x="{px}" y="{py}" width="{pw_}" height="{ph}" rx="18" fill="#FFFFFF" stroke="{LINE}" stroke-width="2"/>')
    s.add(f'<circle cx="{px + 24}" cy="{py + 26}" r="5" fill="{PURPLE}" class="pu"/>')
    s.add(s.text(px + 38, py + 31, 'tools/list  ·  dna mcp serve', 'B', 13, INK))
    tools = [('domain_search', 'read', GREEN), ('domain_info', 'read', GREEN), ('dns_add', 'write', CYAN_D),
             ('dns_delete', 'destructive', AMBER), ('domain_register', 'billable', RED)]
    y = py + 70
    for i, (name, cls, color) in enumerate(tools):
        st = 0.3 + i * 0.25
        s.style(f'.tl{i}{{animation:tl{i} {T}s linear infinite}}@keyframes tl{i}{{0%,{pct(st, T)}{{opacity:0;transform:translateX(-8px)}}{pct(st + 0.3, T)},94%{{opacity:1;transform:none}}98%,100%{{opacity:0}}}}')
        bw = width('B', cls, 11) + 16
        s.add(f'<g class="tl{i}"><rect x="{px + 18}" y="{y - 17}" width="{pw_ - 36}" height="26" rx="8" fill="{SOFT}"/>'
              + s.text(px + 30, y + 0.5, name, 'R', 13, INK)
              + f'<rect x="{px + pw_ - 30 - bw:.1f}" y="{y - 13}" width="{bw:.1f}" height="18" rx="9" fill="{color}"/>'
              + s.text(px + pw_ - 30 - bw / 2, y, cls, 'B', 11, '#FFFFFF', anchor='middle') + '</g>')
        y += 31
    msz = fit(lang.N, t['mcp_more'], 12, pw_ - 48, 9)
    s.add(s.text(px + 30, py + ph - 14, t['mcp_more'], lang.N, msz, MUTED, rtl=lang.rtl))

    cx, cy, cw, ch = 476, 24, 700, 252
    s.add(f'<rect x="{cx}" y="{cy}" width="{cw}" height="{ch}" rx="18" fill="{TERM}"/>')
    s.add(s.text(cx + 24, cy + 31, 'tools/call', 'B', 13, TERM_TXT))
    s.add(s.text(cx + 112, cy + 31, 'domain_register', 'R', 13, TERM_DIM))
    rows = [
        ('→', '{"domainName":"brandname.com","period":1}', TERM_TXT, 1.8, 'R'),
        ('←', '{"error":{"kind":"confirmation_required",', AMBER_L, 3.0, 'R'),
        (' ', '  "preview":{"method":"POST","path":"/api/v1/domains/register"}}}', AMBER_L, 3.2, 'R'),
        ('◆', t['mcp_ask'], '#B9AEFF', 4.6, lang.chat),
        ('→', '{"domainName":"brandname.com","period":1,"confirm":true}', TERM_TXT, 6.0, 'R'),
        ('←', '{"success":true,"status":"Active","expirationDate":"2027-11-19"}', GREEN, 7.4, 'R'),
    ]
    y = cy + 70
    for i, (arrow, text, color, st, tcls) in enumerate(rows):
        s.style(f'.cr{i}{{animation:cr{i} {T}s linear infinite}}@keyframes cr{i}{{0%,{pct(st, T)}{{opacity:0;transform:translateY(4px)}}{pct(st + 0.3, T)},94%{{opacity:1;transform:none}}98%,100%{{opacity:0}}}}')
        acol = GREEN if arrow == '→' else (CYAN if arrow == '←' else '#B9AEFF')
        size = fit(tcls, text, 13, cw - 72, 9)
        rtl = lang.rtl and tcls == lang.chat
        s.add(f'<g class="cr{i}">' + (s.text(cx + 24, y, arrow, 'B', 14, acol) if arrow.strip() else '')
              + s.text(cx + 48, y, text, tcls, size, color, rtl=rtl) + '</g>')
        y += 30
    return write(lang, 'mcp.svg', s)


# --------------------------------------------------------------------------- language selector pills

def star(cx, cy, r, fill):
    import math
    pts = []
    for k in range(10):
        rad = r if k % 2 == 0 else r * 0.42
        a = -math.pi / 2 + k * math.pi / 5
        pts.append(f'{cx + rad * math.cos(a):.2f},{cy + rad * math.sin(a):.2f}')
    return f'<polygon points="{" ".join(pts)}" fill="{fill}"/>'


def flag(code):
    """A simplified 22x16 flag."""
    w, h = 22, 16
    third = h / 3
    if code == 'tr':
        return (f'<rect width="{w}" height="{h}" fill="#E30A17"/><circle cx="8.6" cy="8" r="4.3" fill="#fff"/>'
                f'<circle cx="9.7" cy="8" r="3.45" fill="#E30A17"/>' + star(13.6, 8, 2.1, '#fff'))
    if code == 'en':
        out = [f'<rect width="{w}" height="{h}" fill="#fff"/>']
        sh = h / 13
        for k in range(0, 13, 2):
            out.append(f'<rect y="{k * sh:.2f}" width="{w}" height="{sh:.2f}" fill="#B22234"/>')
        out.append(f'<rect width="9.6" height="{7 * sh:.2f}" fill="#3C3B6E"/>')
        for (a, b) in [(2, 1.8), (4.8, 1.8), (7.6, 1.8), (3.4, 3.8), (6.2, 3.8), (2, 5.8), (4.8, 5.8), (7.6, 5.8)]:
            out.append(f'<circle cx="{a}" cy="{b}" r=".55" fill="#fff"/>')
        return ''.join(out)
    if code == 'de':
        return ''.join(f'<rect y="{k * third:.2f}" width="{w}" height="{third + .1:.2f}" fill="{c}"/>' for k, c in enumerate(['#000', '#DD0000', '#FFCE00']))
    if code == 'nl':
        return ''.join(f'<rect y="{k * third:.2f}" width="{w}" height="{third + .1:.2f}" fill="{c}"/>' for k, c in enumerate(['#AE1C28', '#fff', '#21468B']))
    if code == 'az':
        return (''.join(f'<rect y="{k * third:.2f}" width="{w}" height="{third + .1:.2f}" fill="{c}"/>' for k, c in enumerate(['#00B5E2', '#EF3340', '#509E2F']))
                + '<circle cx="10.2" cy="8" r="2.3" fill="#fff"/><circle cx="10.9" cy="8" r="1.85" fill="#EF3340"/>' + star(13.1, 8, 1.2, '#fff'))
    if code == 'fr':
        return ''.join(f'<rect x="{k * w / 3:.2f}" width="{w / 3 + .1:.2f}" height="{h}" fill="{c}"/>' for k, c in enumerate(['#0055A4', '#fff', '#EF4135']))
    if code == 'it':
        return ''.join(f'<rect x="{k * w / 3:.2f}" width="{w / 3 + .1:.2f}" height="{h}" fill="{c}"/>' for k, c in enumerate(['#009246', '#fff', '#CE2B37']))
    if code == 'cn':
        return (f'<rect width="{w}" height="{h}" fill="#EE1C25"/>' + star(4.6, 4.4, 2.7, '#FFDE00')
                + star(8.6, 1.9, .8, '#FFDE00') + star(10.1, 3.6, .8, '#FFDE00') + star(10.1, 5.9, .8, '#FFDE00') + star(8.6, 7.6, .8, '#FFDE00'))
    if code == 'sa':
        return (f'<rect width="{w}" height="{h}" fill="#006C35"/><rect x="5" y="4.6" width="12" height="2.2" rx="1.1" fill="#fff" fill-opacity=".92"/>'
                '<rect x="5.5" y="10" width="11" height="1.3" rx=".6" fill="#fff"/><rect x="15.4" y="9.2" width="1.3" height="2.9" rx=".5" fill="#fff"/>')
    raise ValueError(code)


def lang_pills():
    out_dir = os.path.join(ROOT, 'lang')
    os.makedirs(out_dir, exist_ok=True)
    codes = {'tr': 'TR', 'en': 'EN', 'de': 'DE', 'sa': 'SA', 'nl': 'NL', 'az': 'AZ', 'cn': 'CN', 'fr': 'FR', 'it': 'IT'}
    for code in LANG_ORDER:
        name = LANG_PILLS[code][0]
        ncls = {'sa': 'AS', 'cn': 'CS'}.get(code, 'S')
        for active in (False, True):
            label = codes[code]
            h = 32
            lw = width('B', label, 13)
            nwid = width(ncls, name, 13) if active else 0
            w = 12 + 22 + 8 + lw + (10 + nwid if active else 0) + 14
            s = SVG(round(w), h, name)
            s.style('.sw{animation:sw 4.5s ease-in-out infinite}@keyframes sw{0%,55%{transform:none}100%{transform:translateX(' + f'{w + 140:.0f}' + 'px)}}')
            s.deff(f'<linearGradient id="pg" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="{GREEN_D}"/><stop offset="1" stop-color="{GREEN}"/></linearGradient>'
                   f'<clipPath id="pc"><rect width="{w:.1f}" height="{h}" rx="{h / 2}"/></clipPath>'
                   '<clipPath id="fc"><rect x="12" y="8" width="22" height="16" rx="3.5"/></clipPath>'
                   '<linearGradient id="ps"><stop offset="0" stop-color="#fff" stop-opacity="0"/><stop offset=".5" stop-color="#fff" stop-opacity=".35"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></linearGradient>')
            if active:
                s.add(f'<rect width="{w:.1f}" height="{h}" rx="{h / 2}" fill="url(#pg)"/>')
            else:
                s.add(f'<rect x=".75" y=".75" width="{w - 1.5:.1f}" height="{h - 1.5}" rx="{(h - 1.5) / 2}" fill="#FFFFFF" stroke="{LINE}" stroke-width="1.5"/>')
            s.add(f'<g clip-path="url(#fc)"><g transform="translate(12 8)">{flag(code)}</g></g>'
                  f'<rect x="12" y="8" width="22" height="16" rx="3.5" fill="none" stroke="#000" stroke-opacity=".12"/>')
            txt = '#FFFFFF' if active else INK
            s.add(s.text(42, 21, label, 'B', 13, txt))
            if active:
                s.add(s.text(42 + lw + 10, 21, name, ncls, 13, txt, rtl=code == 'sa'))
                s.add(f'<g clip-path="url(#pc)"><rect x="-70" width="50" height="{h}" fill="url(#ps)" class="sw"/></g>')
            fname = f'{code}-active.svg' if active else f'{code}.svg'
            with open(os.path.join(out_dir, fname), 'w', encoding='utf-8', newline='\n') as f:
                f.write(s.render())
    print('lang pills: 18 files')


# --------------------------------------------------------------------------- section icons (30x30)

def icon_svg(name, label, inner, css=''):
    s = SVG(32, 32, label)
    s.style(BASE_CSS + css)
    s.add(f'<rect x="1.5" y="1.5" width="29" height="29" rx="8" fill="#FFFFFF" stroke="{CYAN_D}" stroke-width="1.8"/>')
    s.add(inner)
    with open(os.path.join(ROOT, name), 'w', encoding='utf-8', newline='\n') as f:
        f.write(s.render())


def icons():
    c, g = CYAN_D, GREEN_D
    icon_svg('h-agent.svg', 'AI agents',
             f'<rect x="8" y="11" width="16" height="12" rx="4" fill="none" stroke="{c}" stroke-width="1.8"/>'
             f'<path d="M16 11V7.5" stroke="{c}" stroke-width="1.8" stroke-linecap="round"/><circle cx="16" cy="6.5" r="1.8" fill="{g}" class="pu"/>'
             f'<g class="bl" style="animation-duration:3s"><circle cx="13" cy="17" r="1.6" fill="{c}"/><circle cx="19" cy="17" r="1.6" fill="{c}"/></g>')
    icon_svg('h-install.svg', 'Install',
             f'<path d="M9 21v3h14v-3" fill="none" stroke="{c}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>'
             f'<g class="dl"><path d="M16 7v11M11.5 13.5 16 18l4.5-4.5" fill="none" stroke="{g}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></g>',
             '.dl{animation:dl 1.8s ease-in-out infinite}@keyframes dl{50%{transform:translateY(2.5px)}}')
    icon_svg('h-shield.svg', 'Safety',
             f'<path d="M16 6.5 23 9.3v5.2c0 4.3-3 7.2-7 8.7-4-1.5-7-4.4-7-8.7V9.3z" fill="none" stroke="{c}" stroke-width="1.8" stroke-linejoin="round"/>'
             f'<path d="M12.6 15.2 15 17.6l4.4-4.8" fill="none" stroke="{g}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="pu"/>')
    icon_svg('h-terminal.svg', 'Terminal',
             f'<path d="M9 11.5 13 15.5 9 19.5" fill="none" stroke="{c}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>'
             f'<rect x="15" y="18.5" width="8" height="2.4" rx="1.2" fill="{g}" class="bl"/>')
    icon_svg('h-mcp.svg', 'MCP',
             f'<path d="M12.5 7.5v4M19.5 7.5v4" stroke="{c}" stroke-width="1.9" stroke-linecap="round"/>'
             f'<path d="M10 11.5h12v3.2a6 6 0 0 1-12 0z" fill="none" stroke="{c}" stroke-width="1.8" stroke-linejoin="round"/>'
             f'<path d="M16 20.7v3.8" stroke="{g}" stroke-width="2" stroke-linecap="round" class="pu"/>')
    icon_svg('h-commands.svg', 'Commands',
             f'<g class="cm"><circle cx="10.5" cy="11" r="1.5" fill="{g}"/><circle cx="10.5" cy="16" r="1.5" fill="{c}"/><circle cx="10.5" cy="21" r="1.5" fill="{c}"/></g>'
             f'<path d="M14 11h8M14 16h8M14 21h6" stroke="{c}" stroke-width="1.8" stroke-linecap="round"/>',
             '.cm circle{animation:cm 3s steps(1) infinite}.cm circle:nth-child(2){animation-delay:1s}.cm circle:nth-child(3){animation-delay:2s}'
             '@keyframes cm{0%{fill:' + g + '}33%,100%{fill:' + c + '}}')
    icon_svg('h-prompt.svg', 'Prompts',
             f'<path d="M8.5 10.5a2.5 2.5 0 0 1 2.5-2.5h10a2.5 2.5 0 0 1 2.5 2.5v7a2.5 2.5 0 0 1-2.5 2.5h-5.5l-4 3.5V20h-.5a2.5 2.5 0 0 1-2.5-2.5z" fill="none" stroke="{c}" stroke-width="1.7" stroke-linejoin="round"/>'
             f'<g class="tw"><path d="M16 10.6v5.8M13.1 13.5h5.8" stroke="{g}" stroke-width="1.8" stroke-linecap="round"/></g>',
             '.tw{transform-box:fill-box;transform-origin:center;animation:tw 2.4s ease-in-out infinite}@keyframes tw{50%{transform:rotate(45deg) scale(.7);opacity:.55}}')
    icon_svg('h-docs.svg', 'Documentation',
             f'<path d="M9 9.5h5.5c1 0 1.5.6 1.5 1.5v12c0-.9-.6-1.5-1.5-1.5H9zM23 9.5h-5.5c-1 0-1.5.6-1.5 1.5v12c0-.9.6-1.5 1.5-1.5H23z" fill="none" stroke="{c}" stroke-width="1.6" stroke-linejoin="round"/>'
             f'<path d="M11 13h3M11 16h3M18 13h3" stroke="{g}" stroke-width="1.4" stroke-linecap="round" class="pu"/>')
    icon_svg('h-output.svg', 'Output',
             f'<path d="M12 9.5c-2 0-2.5 1-2.5 2.5v1.5c0 1-.6 2-1.5 2.5.9.5 1.5 1.5 1.5 2.5V20c0 1.5.5 2.5 2.5 2.5M20 9.5c2 0 2.5 1 2.5 2.5v1.5c0 1 .6 2 1.5 2.5-.9.5-1.5 1.5-1.5 2.5V20c0 1.5-.5 2.5-2.5 2.5" fill="none" stroke="{c}" stroke-width="1.7" stroke-linecap="round"/>'
             f'<circle cx="16" cy="16" r="1.8" fill="{g}" class="pu"/>')
    print('section icons: 9 files')


if __name__ == '__main__':
    only = os.environ.get('LANGS', '').split(',') if os.environ.get('LANGS') else LANG_ORDER
    for code in only:
        lang = Lang(code)
        sizes = [f(lang) for f in (hero, stats, flow, safety, mcp)]
        print(f'{code}: ' + ', '.join(f'{n} {b / 1024:.0f}KB' for n, b in zip(['hero', 'stats', 'flow', 'safety', 'mcp'], sizes)))
    lang_pills()
    icons()
