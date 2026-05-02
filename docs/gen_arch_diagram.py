#!/usr/bin/env python3
"""Generate engram-go three-phase architecture evolution diagram."""

W, H = 1800, 760

NAVY      = "#0F172A"
NAVY_LT   = "#243B55"
PANEL_BG  = "#1e293b"
GO_SVC    = "#1e3a5f"
DB_CLR    = "#14332a"
INFRA_CLR = "#2a1a3a"
EXTERNAL  = "#2d2a14"
BORDER    = "#334155"
GOLD      = "#FFAF18"
GOLD_DIM  = "#D4A574"
CREAM     = "#f8fafc"
GRAY      = "#94a3b8"
TAG_TEXT  = "#0F172A"

FONT_DISP = "Kufam, 'Playfair Display', Georgia, serif"
FONT_BODY = "Inter, 'Source Sans Pro', 'Segoe UI', sans-serif"


def r(x, y, w, h, fill, stroke=None, sw=1, rx=0, dash=None, fopacity=None):
    s = f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="{fill}" rx="{rx}"'
    if stroke:
        s += f' stroke="{stroke}" stroke-width="{sw}"'
    if dash:
        s += f' stroke-dasharray="{dash}"'
    if fopacity is not None:
        s += f' fill-opacity="{fopacity}"'
    return s + '/>'


def t(x, y, content, size, fill, weight="normal", family=None, anchor="start"):
    fam = family or FONT_BODY
    return (f'<text x="{x}" y="{y}" font-size="{size}" fill="{fill}" '
            f'font-weight="{weight}" font-family="{fam}" text-anchor="{anchor}">'
            f'{content}</text>')


def ln(x1, y1, x2, y2, stroke, sw=1, dash=None):
    s = f'<line x1="{x1}" y1="{y1}" x2="{x2}" y2="{y2}" stroke="{stroke}" stroke-width="{sw}"'
    if dash:
        s += f' stroke-dasharray="{dash}"'
    return s + '/>'


def horiz_arrow(x1, x2, yc, label):
    shaft_h = 8
    tip_w = 12
    shaft_w = x2 - x1 - tip_w
    sy = yc - shaft_h // 2
    return '\n'.join([
        r(x1, sy, shaft_w, shaft_h, GOLD),
        (f'<polygon points="{x1+shaft_w},{sy-4} {x1+shaft_w},{sy+shaft_h+4} {x2},{yc}" '
         f'fill="{GOLD}"/>'),
        t((x1 + x2) // 2, sy - 7, label, 11, GOLD_DIM, anchor="middle"),
    ])


def comp_box(cx, cy, cw, ch, fill, label, details, dashed=False):
    dash = "4,3" if dashed else None
    lines_out = [
        r(cx, cy, cw, ch, fill, stroke=BORDER, sw=1, rx=6, dash=dash),
        t(cx + 14, cy + 21, label, 13, CREAM, weight="bold"),
    ]
    for i, d in enumerate(details):
        lines_out.append(t(cx + 14, cy + 37 + i * 15, d, 11, GRAY))
    return '\n'.join(lines_out)


def draw_panel(px, py, pw, ph, tag, subtitle, components, badge="", footer=""):
    out = []

    # Panel background
    out.append(r(px, py, pw, ph, PANEL_BG, stroke=BORDER, sw=1, rx=8))

    # Optional overlay tint
    if badge == "K8S":
        out.append(r(px+10, py+10, pw-20, ph-20, "#0d1f0d", stroke=BORDER,
                     sw=1, rx=6, dash="5,4", fopacity=0.4))
        badge_label = "K8s Cluster"
    elif badge == "AWS":
        out.append(r(px+10, py+10, pw-20, ph-20, "#0a1a2a", stroke=GOLD_DIM,
                     sw=1, rx=6, dash="6,4", fopacity=0.5))
        badge_label = "AWS Cloud"
    else:
        badge_label = ""

    # Phase tag
    tag_x = px + 14
    tag_y = py + 14
    tag_w = max(len(tag) * 8 + 18, 54)
    tag_h = 22
    out.append(r(tag_x, tag_y, tag_w, tag_h, GOLD, rx=4))
    out.append(t(tag_x + tag_w // 2, tag_y + 15, tag, 11, TAG_TEXT,
                 weight="bold", anchor="middle"))

    # Top-right badge label (K8s Cluster / AWS Cloud)
    if badge_label:
        out.append(t(px + pw - 14, tag_y + 15, badge_label, 11, GRAY, anchor="end"))

    # Subtitle
    sub_y = tag_y + tag_h + 14
    out.append(t(tag_x, sub_y, subtitle, 11, GRAY))

    # Divider
    div_y = sub_y + 18
    out.append(ln(px + 14, div_y, px + pw - 14, div_y, BORDER))

    # Component layout
    cx = px + 14
    cw = pw - 28
    footer_reserve = 28 if footer else 0
    comp_start = div_y + 10
    avail = (py + ph) - comp_start - footer_reserve - 12
    n = len(components)
    gap = 8
    ch = (avail - gap * (n - 1)) // n

    for i, comp in enumerate(components):
        cy = comp_start + i * (ch + gap)
        out.append(comp_box(cx, cy, cw, ch,
                            comp.get('fill', GO_SVC),
                            comp['label'],
                            comp.get('details', []),
                            dashed=comp.get('ext', False)))
        # Connector tick between boxes
        if i < n - 1 and not comp.get('ext', False):
            mid = cx + cw // 2
            out.append(ln(mid, cy + ch, mid, cy + ch + gap, BORDER))

    if footer:
        out.append(t(px + 14, py + ph - 10, footer, 11, GOLD_DIM))

    return '\n'.join(out)


# =================== LAYOUT ===================
PY = 80
PH = 668

P1X, P1W = 30,   500
P2X, P2W = 600,  500
P3X, P3W = 1170, 600

# =================== PANELS ===================
p1_comps = [
    {'label': 'Developer Client',
     'details': ['MCP over SSE transport', 'Bearer: ENGRAM_API_KEY'],
     'fill': NAVY_LT},
    {'label': 'engram-go  :8788',
     'details': ['43 MCP Tools  /  4-Signal Search',
                 'BM25 + Vector + Recency + Graph',
                 'Workers: summarize, re-embed, audit, weights'],
     'fill': GO_SVC},
    {'label': 'PostgreSQL + pgvector',
     'details': ['1024-dim vectors  /  HNSW index',
                 'Single tenant  /  No RLS'],
     'fill': DB_CLR},
    {'label': 'Ollama (local)',
     'details': ['configured model  /  localhost:11434',
                 'Local inference  /  no network dep'],
     'fill': EXTERNAL, 'ext': True},
]

p2_comps = [
    {'label': 'Ingress (Traefik)',
     'details': ['TLS termination  /  IngressRoute',
                 'Route by host or path'],
     'fill': INFRA_CLR},
    {'label': 'engram-gateway  (Go)',
     'details': ['Control Plane  /  REST API',
                 'Tenant resolution  /  X-Tenant-ID inject'],
     'fill': GO_SVC},
    {'label': 'engram-go  x2 pods',
     'details': ['Data Plane  /  43 MCP Tools',
                 'Stateless  /  HPA ready'],
     'fill': GO_SVC},
    {'label': 'PostgreSQL StatefulSet',
     'details': ['Row-Level Security (RLS)',
                 'Per-tenant isolation  /  pgvector HNSW'],
     'fill': DB_CLR},
    {'label': 'Ollama (external)',
     'details': ['leviathan.petersimmons.com:11434',
                 'configured model  /  1024-dim'],
     'fill': EXTERNAL, 'ext': True},
]

p3_comps = [
    {'label': 'Rust Auth Gateway',
     'details': ['OIDC / JWT validation',
                 'Infisical Cloud secrets  /  Rate limiting'],
     'fill': INFRA_CLR},
    {'label': 'ECS / Fargate',
     'details': ['engram-go fleet  /  Auto-scaling',
                 'ECR images  /  Trivy scanned  /  Chainguard base'],
     'fill': GO_SVC},
    {'label': 'RDS Aurora PostgreSQL',
     'details': ['Serverless v2  /  ACU 0-32',
                 'pgvector + RLS  /  RDS Proxy'],
     'fill': DB_CLR},
    {'label': 'S3 Object Lock  +  ECR',
     'details': ['WORM audit logs  /  Compliance',
                 'Container registry  /  Image scanning'],
     'fill': NAVY_LT},
    {'label': 'Ollama  /  OpenAI Embeddings',
     'details': ['Pluggable embedder interface',
                 'leviathan or cloud API fallback'],
     'fill': EXTERNAL, 'ext': True},
]

parts = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {W} {H}" width="100%">',
    '',
    '<!-- =================== CANVAS =================== -->',
    r(0, 0, W, H, NAVY),
    '',
    '<!-- =================== TITLE BAND =================== -->',
    t(36, 54, 'engram-go', 26, CREAM, weight='bold', family=FONT_DISP),
    t(172, 54, 'Architecture Evolution', 22, GRAY, family=FONT_DISP),
    t(W - 30, 44, 'Three-Phase Roadmap', 11, GRAY, anchor='end'),
    t(W - 30, 60, 'OSS Today  >  K8s Phase 1  >  AWS SaaS Phase 3', 11, GOLD_DIM, anchor='end'),
    ln(30, 70, W - 30, 70, BORDER),
    '',
    '<!-- =================== PANEL 1: TODAY =================== -->',
    draw_panel(P1X, PY, P1W, PH, 'TODAY',
               'GitHub OSS  /  Docker Compose  /  Single User',
               p1_comps,
               footer='v3.1   43 MB   200ms cold start'),
    '',
    '<!-- =================== ARROW 1 =================== -->',
    horiz_arrow(P1X + P1W + 4, P2X - 4, PY + PH // 2, 'Multi-tenant + RLS'),
    '',
    '<!-- =================== PANEL 2: PHASE 1 =================== -->',
    draw_panel(P2X, PY, P2W, PH, 'PHASE 1',
               'Kubernetes  /  Multi-Tenant  /  Homelab',
               p2_comps,
               badge='K8S',
               footer='Migrations 014 + 015  /  Zero-downtime cutover'),
    '',
    '<!-- =================== ARROW 2 =================== -->',
    horiz_arrow(P2X + P2W + 4, P3X - 4, PY + PH // 2, 'AWS + SaaS Scale'),
    '',
    '<!-- =================== PANEL 3: PHASE 3 =================== -->',
    draw_panel(P3X, PY, P3W, PH, 'PHASE 3',
               'AWS SaaS  /  1000s of Tenants  /  Production',
               p3_comps,
               badge='AWS',
               footer='Stage 0: $0-15/mo  /  Stage 2: $400-1200/mo  /  HIPAA-ready'),
    '',
    '</svg>',
]

output = '/home/psimmons/projects/engram-go/docs/architecture-evolution.svg'
svg_content = '\n'.join(parts)

with open(output, 'w') as f:
    f.write(svg_content)

print(f'Written: {output}')
print(f'Bytes: {len(svg_content)}')
