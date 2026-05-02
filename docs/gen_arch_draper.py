#!/usr/bin/env python3
"""engram-go architecture diagram — Don Draper / 1960s ad agency edition.

Warm cream paper stock. Charcoal ink. Sterling Cooper burgundy and slate.
Three acts. One bold headline. No apologies.
"""

W, H = 1800, 800

PAPER      = "#F3EAD4"   # warm cream stock
PAPER_DK   = "#E9DFC5"   # component box tint
INK        = "#1A1210"   # near-black, warm charcoal
BURGUNDY   = "#78191A"   # Sterling Cooper deep red
SLATE      = "#284261"   # 1960s corporate slate blue
AMBER      = "#B8841C"   # whiskey brass
AMBER_LT   = "#D9A830"   # lighter amber for labels on dark headers
MID        = "#7A6E5E"   # warm mid-gray for secondary text
RULE_CLR   = "#C8B89A"   # warm tan for interior rules
CREAM_HDR  = "#F3EAD4"   # cream text on dark headers

SERIF = "Georgia, 'Times New Roman', serif"
SANS  = "Arial, Helvetica, sans-serif"


def esc(s):
    return (s.replace("&", "&amp;")
             .replace("<", "&lt;")
             .replace(">", "&gt;"))


def R(x, y, w, h, fill, stroke=None, sw=1, rx=0, dash=None):
    s = (f'<rect x="{x}" y="{y}" width="{w}" height="{h}" '
         f'fill="{fill}" rx="{rx}"')
    if stroke:
        s += f' stroke="{stroke}" stroke-width="{sw}"'
    if dash:
        s += f' stroke-dasharray="{dash}"'
    return s + '/>'


def T(x, y, content, size, fill, weight="normal", family=None,
      anchor="start", italic=False, spacing=None):
    fam = family or SERIF
    style_parts = []
    if italic:
        style_parts.append("font-style:italic")
    if spacing:
        style_parts.append(f"letter-spacing:{spacing}px")
    style_str = f' style="{";".join(style_parts)}"' if style_parts else ""
    return (f'<text x="{x}" y="{y}" font-size="{size}" fill="{fill}" '
            f'font-weight="{weight}" font-family="{fam}" '
            f'text-anchor="{anchor}"{style_str}>{esc(content)}</text>')


def L(x1, y1, x2, y2, stroke, sw=1, dash=None):
    s = (f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" '
         f'stroke="{stroke}" stroke-width="{sw}"')
    if dash:
        s += f' stroke-dasharray="{dash}"'
    return s + '/>'


def arrow(x1, x2, yc, label):
    tip = 14
    shaft_h = 5
    shaft_w = x2 - x1 - tip
    sy = yc - shaft_h // 2
    return '\n'.join([
        R(x1, sy, shaft_w, shaft_h, INK),
        (f'<polygon points="{x1+shaft_w},{yc-9} '
         f'{x1+shaft_w},{yc+9} {x2},{yc}" fill="{INK}"/>'),
        T((x1+x2)//2, sy - 9, label, 11, MID, anchor="middle",
          italic=True, spacing=1),
    ])


def comp_box(cx, cy, cw, ch, label, details, accent, dashed=False):
    dash = "7,4" if dashed else None
    fill = PAPER if dashed else PAPER_DK
    stroke = RULE_CLR if dashed else INK
    sw = 1
    parts = [R(cx, cy, cw, ch, fill, stroke=stroke, sw=sw, rx=2, dash=dash)]
    if not dashed:
        # Thin accent stripe at top of box
        parts.append(R(cx, cy, cw, 3, accent, rx=0))
    # Label
    lbl_fill = MID if dashed else INK
    parts.append(T(cx + 12, cy + 22, label.upper(), 12, lbl_fill,
                   weight="bold", family=SANS, spacing=1))
    # Rule under label
    parts.append(L(cx + 12, cy + 29, cx + cw - 12, cy + 29, RULE_CLR))
    # Details
    detail_fill = RULE_CLR if dashed else MID
    for i, d in enumerate(details):
        parts.append(T(cx + 12, cy + 44 + i * 16, d, 11, detail_fill,
                       italic=True))
    return '\n'.join(parts)


def draw_column(px, py, pw, ph, act, title, subtitle, color, comps, footer=""):
    HDR = 82
    out = []

    # Column border
    out.append(R(px, py, pw, ph, PAPER, stroke=INK, sw=1, rx=0))

    # Header bar
    out.append(R(px, py, pw, HDR, color, rx=0))

    # Act label (amber, small caps feel via uppercase + spacing)
    out.append(T(px + 16, py + 18, act, 11, AMBER_LT, weight="bold",
                 family=SANS, spacing=3))

    # Title (large cream serif)
    out.append(T(px + 16, py + 50, title, 24, CREAM_HDR, weight="bold"))

    # Subtitle (small, muted cream, uppercase)
    out.append(T(px + 16, py + 70, subtitle, 11, "#B09870",
                 family=SANS, spacing=2))

    # Component area
    comp_y0 = py + HDR + 10
    footer_h = 26 if footer else 0
    avail = ph - HDR - 10 - footer_h - 10
    cx = px + 12
    cw = pw - 24
    gap = 8
    n = len(comps)
    ch = (avail - gap * (n - 1)) // n

    for i, comp in enumerate(comps):
        cy = comp_y0 + i * (ch + gap)
        out.append(comp_box(cx, cy, cw, ch,
                            comp['label'],
                            comp.get('details', []),
                            color,
                            dashed=comp.get('ext', False)))
        # Connector line between non-external boxes
        if i < n - 1 and not comp.get('ext', False):
            mid = cx + cw // 2
            out.append(L(mid, cy + ch, mid, cy + ch + gap, RULE_CLR))

    if footer:
        out.append(T(px + 12, py + ph - 10, footer, 11, MID,
                     italic=True, spacing=0))

    return '\n'.join(out)


# =================== COMPONENT DATA ===================

p1 = [
    {'label': 'Developer Client',
     'details': ['MCP over SSE transport',
                 'Bearer: ENGRAM_API_KEY']},
    {'label': 'engram-go  :8788',
     'details': ['43 MCP Tools  /  4-Signal Search',
                 'BM25 + Vector + Recency + Graph',
                 'Workers: summarize, re-embed, audit, weights']},
    {'label': 'PostgreSQL + pgvector',
     'details': ['1024-dim vectors  /  HNSW index',
                 'Single tenant  /  No RLS']},
    {'label': 'Ollama (local)',
     'details': ['configured model  /  localhost:11434',
                 'Local inference, no external dependency'],
     'ext': True},
]

p2 = [
    {'label': 'Ingress (Traefik)',
     'details': ['TLS termination  /  IngressRoute',
                 'Route by host or path']},
    {'label': 'engram-gateway (Go)',
     'details': ['Control Plane  /  REST API',
                 'Tenant resolution  /  X-Tenant-ID inject']},
    {'label': 'engram-go  x2 pods',
     'details': ['Data Plane  /  43 MCP Tools',
                 'Stateless replicas  /  HPA ready']},
    {'label': 'PostgreSQL StatefulSet',
     'details': ['Row-Level Security (RLS)',
                 'Per-tenant isolation  /  pgvector HNSW']},
    {'label': 'Ollama (external)',
     'details': ['leviathan.petersimmons.com:11434',
                 'configured model  /  1024-dim'],
     'ext': True},
]

p3 = [
    {'label': 'Rust Auth Gateway',
     'details': ['OIDC / JWT validation',
                 'Infisical Cloud secrets  /  Rate limiting']},
    {'label': 'ECS / Fargate',
     'details': ['engram-go fleet  /  Auto-scaling',
                 'ECR images  /  Trivy scanned  /  Chainguard base']},
    {'label': 'RDS Aurora PostgreSQL',
     'details': ['Serverless v2  /  ACU 0-32',
                 'pgvector + RLS  /  RDS Proxy']},
    {'label': 'S3 Object Lock  +  ECR',
     'details': ['WORM audit logs  /  Compliance',
                 'Container registry  /  Image scanning']},
    {'label': 'Ollama  /  OpenAI Embeddings',
     'details': ['Pluggable embedder interface',
                 'leviathan or cloud API fallback'],
     'ext': True},
]

# =================== LAYOUT ===================
PY = 96
PH = 678

P1X, P1W = 40,   450
P2X, P2W = 570,  450
P3X, P3W = 1100, 660

svg = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="100%">',
    '',
    '<!-- =================== CANVAS =================== -->',
    R(0, 0, W, H, PAPER),
    '',
    '<!-- =================== TITLE BAND =================== -->',
    L(40, 16, W-40, 16, INK, sw=3),
    L(40, 20, W-40, 20, AMBER, sw=1),
    T(40, 62, 'BUILT TO SCALE.', 44, INK, weight='bold', spacing=5),
    T(W-40, 54, 'engram-go: An Architecture in Three Acts', 15, MID,
      anchor='end', italic=True),
    T(W-40, 74, '2026', 13, AMBER, weight='bold', anchor='end',
      family=SANS, spacing=4),
    L(40, 86, W-40, 86, INK, sw=1),
    L(40, 89, W-40, 89, RULE_CLR, sw=1),
    '',
    '<!-- =================== COLUMN 1: TODAY =================== -->',
    draw_column(P1X, PY, P1W, PH,
                'TODAY', 'The Workbench', 'LOCAL  /  OPEN SOURCE',
                INK, p1,
                footer='v3.1  |  43 MB  |  200ms cold start'),
    '',
    '<!-- =================== ARROW 1 =================== -->',
    arrow(P1X+P1W+6, P2X-6, PY+PH//2, 'Kubernetes + RLS'),
    '',
    '<!-- =================== COLUMN 2: PHASE 1 =================== -->',
    draw_column(P2X, PY, P2W, PH,
                'PHASE ONE', 'The Cluster', 'KUBERNETES  /  MULTI-TENANT',
                SLATE, p2,
                footer='Migrations 014 + 015  |  Zero-downtime cutover'),
    '',
    '<!-- =================== ARROW 2 =================== -->',
    arrow(P2X+P2W+6, P3X-6, PY+PH//2, 'AWS + SaaS Scale'),
    '',
    '<!-- =================== COLUMN 3: PHASE 3 =================== -->',
    draw_column(P3X, PY, P3W, PH,
                'PHASE THREE', 'The Platform', 'AWS SAAS  /  1000s OF TENANTS',
                BURGUNDY, p3,
                footer='Stage 0: $0-15/mo  |  Stage 2: $400-1200/mo  |  HIPAA-ready design'),
    '',
    '<!-- =================== FOOTER RULE =================== -->',
    L(40, H-18, W-40, H-18, INK, sw=1),
    T(40, H-7, 'yourai.com', 11, MID, family=SANS, spacing=2),
    T(W-40, H-7, 'CONFIDENTIAL  /  INTERNAL USE ONLY', 11, MID,
      anchor='end', family=SANS, spacing=2),
    '',
    '</svg>',
]

OUTPUT_SVG = '/home/psimmons/projects/engram-go/docs/architecture-draper.svg'
content = '\n'.join(svg)

with open(OUTPUT_SVG, 'w') as f:
    f.write(content)

print(f'Written: {OUTPUT_SVG}')
print(f'Size: {len(content)} bytes')
