"""Builds final_report_template.pptx (the Laporan Akhir deck) and a PNG
preview of every slide from one layout description.

    python -I build.py              # writes ../final_report_template.pptx
    python -I build.py --preview D  # also writes D/slide1.png ... with sample data

The deck keeps the {{TOKEN}} placeholders of placeholders.go in single text
runs (RenderPPTX replaces them in the slide XML) and gives every photo slot
its own placeholder image, so each slot is a separate ppt/media file that
RenderPPTX overwrites. The media names per slide are printed at the end and
must match slideImageOrder / placeholderOnlySlots in pptx.go.

Requires python-pptx and Pillow. Fonts: Calibri (Windows) for the preview.
"""

import io
import os
import re
import sys

from PIL import Image, ImageDraw, ImageFont
from pptx import Presentation
from pptx.dml.color import RGBColor
from pptx.enum.shapes import MSO_SHAPE
from pptx.enum.text import MSO_ANCHOR, PP_ALIGN
from pptx.oxml.ns import qn
from pptx.util import Emu, Inches, Pt

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "..", "final_report_template.pptx")
LOGO = os.path.join(HERE, "assets", "ksm_logo.png")
ISO = os.path.join(HERE, "assets", "iso_strip.png")

W, H = 13.333, 7.5  # 16:9, inches

# Palette: deep navy + the gold of the KSM logo.
NAVY = "0F2A43"
NAVY_2 = "1C3D5E"
GOLD = "B8893A"
GOLD_LIGHT = "F3EAD8"
INK = "1F2937"
MUTED = "6B7280"
PANEL = "F6F4EF"
LINE = "E5E0D5"
WHITE = "FFFFFF"
BULLET_INDENT = 0.28

# ---------------------------------------------------------------- layout DSL
# Each slide is a list of elements; coordinates in inches.
#   ("rect", x, y, w, h, fill, {"line": hex})
#   ("text", x, y, w, h, paragraphs, {"align": "l|c|r", "anchor": "t|m|b"})
#       paragraphs: [[(text, size, bold, color, {"spc": 200})...], ...] or with
#       a leading dict {"space_after": pt, "line": 1.1}
#   ("image", x, y, w, h, path)
#   ("photo", x, y, w, h, slot_label)


def run(text, size, color=INK, bold=False, **kw):
    return (text, size, bold, color, kw)


def para(*runs, space_after=0, line=1.1, bullet=False):
    return [{"space_after": space_after, "line": line, "bullet": bullet}, *runs]


def chrome(number, regency=True):
    """Header logo + rule and footer certificates + page number."""
    return [
        ("image", 0.5, 0.36, 3.3, 3.3 / 6.736, LOGO),
        ("text", 7.3, 0.42, 5.53, 0.4, [para(run("LAPORAN AKHIR KEGIATAN", 10, GOLD, True, spc=150))]
         + ([para(run("{{REGENCY_LABEL}}  ·  TA {{FISCAL_YEAR}}", 10, MUTED))] if regency else []), {"align": "r"}),
        ("rect", 0.5, 1.0, 12.333, 0.025, GOLD, {}),
        ("image", 0.5, 6.98, 2.3, 2.3 / 8.196, ISO),
        ("text", 8.3, 6.98, 4.53, 0.3, [para(run("PT Kian Santang Muliatama Tbk   ", 9, MUTED), run(f"{number:02d}", 9, NAVY, True))], {"align": "r", "anchor": "m"}),
    ]


def heading(eyebrow, title, y=1.3):
    return [
        ("text", 0.5, y, 12.3, 0.3, [para(run(eyebrow, 11, GOLD, True, spc=200))], {}),
        ("text", 0.5, y + 0.3, 12.3, 0.6, [para(run(title, 28, NAVY, True))], {}),
    ]


def card(x, y, w, h, fill=PANEL):
    return [("rect", x, y, w, h, fill, {}), ("rect", x, y, 0.07, h, GOLD, {})]


def bullet_list(items, size=13):
    return [para(run(item, size, INK), space_after=5, bullet=True) for item in items]


ATTENDEES = [
    "Tim ESDM",
    "Tim Pertamina",
    "{{AGRICULTURE_OFFICE_NAME}}",
    "PT Kian Santang Muliatama Tbk (Penyedia Barang)",
    "{{CONSULTANT_COMPANY_NAME}} (Konsultan Pengawas)",
    "{{MACHINE_MANUFACTURER_NAME}} (Pabrikan Mesin)",
]


def cover():
    return [
        ("rect", 0, 0, 8.1, H, NAVY, {}),
        ("rect", 8.1, 0, 0.08, H, GOLD, {}),
        ("rect", 0.75, 0.9, 0.9, 0.05, GOLD, {}),
        ("text", 0.75, 1.1, 6.9, 0.4, [para(run("LAPORAN AKHIR KEGIATAN", 14, GOLD, True, spc=300))], {}),
        ("text", 0.75, 1.65, 6.9, 2.6, [para(run("Penyediaan dan Pendistribusian Paket Perdana Liquefied Petroleum Gas (LPG) untuk Mesin Pompa Air bagi Petani Sasaran", 28, WHITE, True), line=1.05)], {}),
        ("text", 0.75, 4.25, 6.9, 0.8, [
            para(run("Tahun Anggaran {{FISCAL_YEAR}}", 16, "D6DEE8"), space_after=2),
            para(run("di PT Pertamina Patra Niaga", 16, "D6DEE8")),
        ], {}),
        ("rect", 0.75, 5.35, 6.6, 0.85, NAVY_2, {}),
        ("rect", 0.75, 5.35, 0.07, 0.85, GOLD, {}),
        ("text", 1.05, 5.35, 6.2, 0.85, [para(run("{{REGENCY_LABEL}}", 22, GOLD, True))], {"anchor": "m"}),
        ("text", 8.75, 2.55, 4.1, 0.3, [para(run("PENYEDIA BARANG", 10, MUTED, True, spc=200))], {}),
        ("image", 8.75, 2.9, 4.1, 4.1 / 6.736, LOGO),
        ("rect", 8.75, 3.75, 4.1, 0.015, LINE, {}),
        ("text", 8.75, 3.9, 4.1, 0.9, [para(run("Pendistribusian paket perdana LPG beserta mesin pompa air untuk petani sasaran.", 12, MUTED), line=1.15)], {}),
        ("image", 8.75, 6.75, 3.2, 3.2 / 8.196, ISO),
    ]


def slide_pengiriman():
    left, right = 0.5, 6.78
    y, h, w = 2.4, 4.3, 6.05
    timeline = [
        ("{{DELIVERY_HOSE_DATE}}", "Selang Buang"),
        ("{{DELIVERY_MACHINE_GROUP_DATE}}", "Mesin Pompa, Konkit, Oli, Bracket, Name Plate dan Selang Hisap"),
        ("{{DELIVERY_GAS_CYLINDER_DATE}}", "Tabung Gas 3 Kg"),
    ]
    els = chrome(2) + heading("01  ·  PERSIAPAN", "Pengiriman Barang & Rapat Koordinasi")
    els += card(left, y, w, h)
    els.append(("text", left + 0.35, y + 0.25, w - 0.6, 0.85, [
        para(run("Pengiriman dan Penerimaan Barang", 17, NAVY, True), space_after=3),
        para(run("{{AGRICULTURE_OFFICE_NAME}}", 12, MUTED)),
    ], {}))
    ty = y + 1.25
    for date, items in timeline:
        els.append(("rect", left + 0.38, ty + 0.08, 0.14, 0.14, GOLD, {}))
        els.append(("text", left + 0.7, ty, w - 1.0, 0.95, [
            para(run(date, 14, NAVY, True), space_after=1),
            para(run(items, 12, INK), line=1.1),
        ], {}))
        ty += 0.98
    els += card(right, y, w, h)
    els.append(("text", right + 0.35, y + 0.25, w - 0.6, 3.9, [
        para(run("Rapat Koordinasi", 17, NAVY, True), space_after=3),
        para(run("{{RAKORDA_DATE}}", 14, GOLD, True), space_after=2),
        para(run("{{RAKORDA_LOCATION}}", 12, MUTED), space_after=12),
        para(run("DIHADIRI OLEH", 10, MUTED, True, spc=150), space_after=6),
        *bullet_list(ATTENDEES),
    ], {}))
    return els


def slide_sosialisasi():
    left, right = 0.5, 6.78
    y, h, w = 2.4, 4.3, 6.05
    els = chrome(3) + heading("02  ·  SOSIALISASI", "Sosialisasi & Tim Pelaksana")
    els += card(left, y, w, h)
    els.append(("text", left + 0.35, y + 0.25, w - 0.6, 3.9, [
        para(run("Sosialisasi", 17, NAVY, True), space_after=3),
        para(run("{{SOSIALISASI_DATE}}", 14, GOLD, True), space_after=2),
        para(run("{{SOSIALISASI_LOCATION}}", 12, MUTED), space_after=12),
        para(run("DIHADIRI OLEH", 10, MUTED, True, spc=150), space_after=6),
        *bullet_list(ATTENDEES),
    ], {}))
    els.append(("text", right, y, w, 0.5, [para(run("Anggota Tim Pelaksana", 17, NAVY, True))], {}))
    tiles = [("{{LEADER_COUNT}}", "Leader"), ("{{ADMIN_COUNT}}", "Administrasi"), ("{{TECHNICIAN_COUNT}}", "Teknisi"), ("{{HELPER_COUNT}}", "Helper")]
    tw, th, gap = (w - 0.25) / 2, 1.75, 0.25
    for i, (count, label) in enumerate(tiles):
        tx = right + (i % 2) * (tw + gap)
        ty = y + 0.55 + (i // 2) * (th + gap)
        els.append(("rect", tx, ty, tw, th, WHITE, {"line": LINE}))
        els.append(("rect", tx, ty, tw, 0.06, GOLD if i == 0 else NAVY, {}))
        els.append(("text", tx + 0.3, ty + 0.25, tw - 0.5, th - 0.4, [
            para(run(count, 40, NAVY, True), run("  orang", 13, MUTED), space_after=2),
            para(run(label, 14, INK, True)),
        ], {}))
    return els


def slide_pendistribusian():
    els = chrome(4) + heading("03  ·  PENDISTRIBUSIAN", "Pendistribusian Paket")
    els += card(0.5, 2.4, 12.333, 1.25)
    els.append(("text", 0.85, 2.4, 11.8, 1.25, [
        para(run("TITIK SERAH", 10, MUTED, True, spc=150), space_after=4),
        para(run("{{HANDOVER_LOCATION}}", 18, NAVY, True)),
    ], {"anchor": "m"}))
    y, h = 3.95, 2.7
    tw = (12.333 - 0.5) / 3
    for i, (label, value) in enumerate([("TANGGAL MULAI", "{{DISTRIBUTION_START_DATE}}"), ("TANGGAL SELESAI", "{{DISTRIBUTION_END_DATE}}")]):
        x = 0.5 + i * (tw + 0.25)
        els.append(("rect", x, y, tw, h, WHITE, {"line": LINE}))
        els.append(("rect", x, y, tw, 0.06, NAVY, {}))
        els.append(("text", x + 0.35, y, tw - 0.6, h, [
            para(run(label, 10, MUTED, True, spc=150), space_after=8),
            para(run(value, 24, NAVY, True)),
        ], {"anchor": "m"}))
    x = 0.5 + 2 * (tw + 0.25)
    els.append(("rect", x, y, tw, h, NAVY, {}))
    els.append(("rect", x, y, tw, 0.06, GOLD, {}))
    els.append(("text", x + 0.35, y, tw - 0.6, h, [
        para(run("TOTAL PAKET TERSALURKAN", 10, "D6DEE8", True, spc=150), space_after=4),
        para(run("{{DISTRIBUTION_TOTAL}}", 54, GOLD, True), run("  Unit", 18, WHITE, True)),
    ], {"anchor": "m"}))
    return els


def doc_slide(number, title, place, date, layout):
    """Left info panel and a photo grid on the right (frames are 4:3)."""
    els = chrome(number)
    px, py, pw, ph = 0.5, 1.3, 3.4, 5.4
    els.append(("rect", px, py, pw, ph, NAVY, {}))
    els.append(("rect", px, py, pw, 0.06, GOLD, {}))
    lines = [
        para(run("DOKUMENTASI", 11, GOLD, True, spc=200), space_after=10),
        para(run(title, 22, WHITE, True), space_after=16, line=1.05),
        para(run(place, 13, "D6DEE8"), space_after=10, line=1.15),
    ]
    if date:
        lines.append(para(run("TANGGAL", 9, "A9B6C6", True, spc=150), space_after=2))
        for part in date if isinstance(date, list) else [date]:
            lines.append(para(run(part, 14, GOLD, True), space_after=1))
    els.append(("text", px + 0.3, py + 0.35, pw - 0.6, ph - 0.6, lines, {}))
    ax, ay, aw, ah, g = 4.15, 1.3, 8.68, 5.4, 0.2
    frames = []
    if layout == "big+2":
        hb = (aw - g + 2 / 3 * g) / 2  # 4/3 hb + g + 4/3 (hb-g)/2 = aw
        wb = 4 / 3 * hb
        hs = (hb - g) / 2
        ws = 4 / 3 * hs
        oy = ay + (ah - hb) / 2
        frames = [(ax, oy, wb, hb), (ax + wb + g, oy, ws, hs), (ax + wb + g, oy + hs + g, ws, hs)]
    elif layout == "2":
        fw = (aw - g) / 2
        fh = fw * 3 / 4
        oy = ay + (ah - fh) / 2
        frames = [(ax, oy, fw, fh), (ax + fw + g, oy, fw, fh)]
    elif layout == "2x2":
        fh = (ah - g) / 2
        fw = fh * 4 / 3
        ox = ax + (aw - 2 * fw - g) / 2
        frames = [(ox + c * (fw + g), ay + r * (fh + g), fw, fh) for r in range(2) for c in range(2)]
    for i, (x, y, w, h) in enumerate(frames):
        els.append(("photo", x, y, w, h, f"{number}-{i + 1}"))
    return els


def closing():
    return [
        ("rect", 0, 0, W, H, NAVY, {}),
        ("rect", 0, H - 1.55, W, 1.55, WHITE, {}),
        ("rect", 0, H - 1.6, W, 0.05, GOLD, {}),
        ("rect", W / 2 - 0.45, 2.0, 0.9, 0.05, GOLD, {}),
        ("text", 1, 2.25, W - 2, 1.2, [para(run("Terima Kasih", 54, WHITE, True))], {"align": "c"}),
        ("text", 1, 3.45, W - 2, 0.9, [
            para(run("Laporan Akhir Kegiatan  ·  {{REGENCY_LABEL}}", 16, "D6DEE8"), space_after=2),
            para(run("Tahun Anggaran {{FISCAL_YEAR}}", 16, "D6DEE8")),
        ], {"align": "c"}),
        ("image", W / 2 - 1.9, H - 1.2, 3.8, 3.8 / 6.736, LOGO),
    ]


SLIDES = [
    cover(),
    slide_pengiriman(),
    slide_sosialisasi(),
    slide_pendistribusian(),
    doc_slide(5, "Pengiriman dan Penerimaan Barang", "{{HANDOVER_LOCATION}}", ["{{DELIVERY_RANGE_START}}", "s.d. {{DELIVERY_RANGE_END}}"], "big+2"),
    doc_slide(6, "Rapat Koordinasi", "{{RAKORDA_LOCATION}}", "{{RAKORDA_DATE}}", "2"),
    doc_slide(7, "Seremoni dan Sosialisasi", "{{SOSIALISASI_LOCATION}}", "{{SOSIALISASI_DATE}}", "2x2"),
    doc_slide(8, "Pendistribusian", "{{HANDOVER_LOCATION}}", "", "big+2"),
    closing(),
]

# ------------------------------------------------------------------ PPTX


def hexcolor(h):
    return RGBColor.from_string(h)


def placeholder_photo(label):
    """A distinct light-gray JPEG per slot (identical bytes would be shared)."""
    img = Image.new("RGB", (1200, 900), (226, 229, 233))
    d = ImageDraw.Draw(img)
    font = ImageFont.truetype("C:/Windows/Fonts/calibri.ttf", 64) if os.path.exists("C:/Windows/Fonts/calibri.ttf") else ImageFont.load_default()
    d.text((600, 450), f"Foto {label}", fill=(150, 156, 165), font=font, anchor="mm")
    buf = io.BytesIO()
    img.save(buf, "JPEG", quality=80)
    buf.seek(0)
    return buf


def add_text(slide, x, y, w, h, paragraphs, opts):
    box = slide.shapes.add_textbox(Inches(x), Inches(y), Inches(w), Inches(h))
    tf = box.text_frame
    tf.word_wrap = True
    tf.margin_left = tf.margin_right = tf.margin_top = tf.margin_bottom = 0
    tf.vertical_anchor = {"t": MSO_ANCHOR.TOP, "m": MSO_ANCHOR.MIDDLE, "b": MSO_ANCHOR.BOTTOM}[opts.get("anchor", "t")]
    for i, p in enumerate(paragraphs):
        meta, runs = p[0], p[1:]
        pp = tf.paragraphs[0] if i == 0 else tf.add_paragraph()
        pp.alignment = {"l": PP_ALIGN.LEFT, "c": PP_ALIGN.CENTER, "r": PP_ALIGN.RIGHT}[opts.get("align", "l")]
        pp.space_after = Pt(meta["space_after"])
        pp.line_spacing = meta["line"]
        if meta.get("bullet"):
            ppr = pp._p.get_or_add_pPr()
            ppr.set("marL", str(Inches(BULLET_INDENT)))
            ppr.set("indent", str(-Inches(BULLET_INDENT)))
            clr = ppr.makeelement(qn("a:buClr"), {})
            srgb = clr.makeelement(qn("a:srgbClr"), {"val": GOLD})
            clr.append(srgb)
            ppr.append(clr)
            ppr.append(ppr.makeelement(qn("a:buFont"), {"typeface": "Arial"}))
            ppr.append(ppr.makeelement(qn("a:buChar"), {"char": "•"}))
        for text, size, bold, color, kw in runs:
            r = pp.add_run()
            r.text = text
            r.font.name = "Calibri"
            r.font.size = Pt(size)
            r.font.bold = bold
            r.font.color.rgb = hexcolor(color)
            if "spc" in kw:
                r.font._element.set("spc", str(kw["spc"]))


def build_pptx():
    prs = Presentation()
    prs.slide_width, prs.slide_height = Inches(W), Inches(H)
    blank = prs.slide_layouts[6]
    media = []
    for elements in SLIDES:
        slide = prs.slides.add_slide(blank)
        photos = []
        for el in elements:
            kind = el[0]
            if kind == "rect":
                _, x, y, w, h, fill, opts = el
                shape = slide.shapes.add_shape(MSO_SHAPE.RECTANGLE, Inches(x), Inches(y), Inches(w), Inches(h))
                shape.fill.solid()
                shape.fill.fore_color.rgb = hexcolor(fill)
                if opts.get("line"):
                    shape.line.color.rgb = hexcolor(opts["line"])
                    shape.line.width = Pt(0.75)
                else:
                    shape.line.fill.background()
                shape.shadow.inherit = False
            elif kind == "text":
                add_text(slide, *el[1:6], el[6])
            elif kind == "image":
                _, x, y, w, h, path = el
                slide.shapes.add_picture(path, Inches(x), Inches(y), Inches(w), Inches(h))
            elif kind == "photo":
                _, x, y, w, h, label = el
                pic = slide.shapes.add_picture(placeholder_photo(label), Inches(x), Inches(y), Inches(w), Inches(h))
                photos.append(pic._element.blipFill.blip.rEmbed)
        rels = slide.part.rels
        media.append([rels[rid].target_part.partname.lstrip("/") for rid in photos])
    prs.save(OUT)
    return media


# --------------------------------------------------------------- preview

SAMPLE = {
    "REGENCY_LABEL": "KABUPATEN TANJUNG JABUNG TIMUR",
    "FISCAL_YEAR": "2026",
    "AGRICULTURE_OFFICE_NAME": "Dinas Tanaman Pangan dan Hortikultura Kabupaten Tanjung Jabung Timur",
    "CONSULTANT_COMPANY_NAME": "PT Bina Karya (Persero)",
    "MACHINE_MANUFACTURER_NAME": "PT Tanaka Engine Indonesia",
    "DELIVERY_HOSE_DATE": "10 November 2026",
    "DELIVERY_MACHINE_GROUP_DATE": "12 November 2026",
    "DELIVERY_GAS_CYLINDER_DATE": "14 November 2026",
    "DELIVERY_RANGE_START": "10 November 2026",
    "DELIVERY_RANGE_END": "14 November 2026",
    "RAKORDA_DATE": "16 November 2026",
    "SOSIALISASI_DATE": "18 November 2026",
    "RAKORDA_LOCATION": "Aula Kantor Bupati Tanjung Jabung Timur",
    "SOSIALISASI_LOCATION": "Balai Desa Rantau Rasau",
    "LEADER_COUNT": "2", "ADMIN_COUNT": "3", "TECHNICIAN_COUNT": "8", "HELPER_COUNT": "12",
    "HANDOVER_LOCATION": "Balai Desa Rantau Rasau, Gudang Desa Sungai Ular, Lapangan Desa Bandar Jaya",
    "DISTRIBUTION_START_DATE": "20 November 2026",
    "DISTRIBUTION_END_DATE": "24 November 2026",
    "DISTRIBUTION_TOTAL": "104",
}
DPI = 120


def fill_sample(text):
    return re.sub(r"\{\{(\w+)\}\}", lambda m: SAMPLE.get(m.group(1), m.group(0)), text)


def font(size, bold):
    path = "C:/Windows/Fonts/calibrib.ttf" if bold else "C:/Windows/Fonts/calibri.ttf"
    return ImageFont.truetype(path, max(1, round(size / 72 * DPI)))


def rgb(h):
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


def px(v):
    return round(v * DPI)


def layout_paragraph(runs, width):
    """Greedy word wrap over mixed runs; returns lines of (word, font, color, size)."""
    words = []
    for text, size, bold, color, _ in runs:
        f = font(size, bold)
        for i, part in enumerate(re.split(r"(\s+)", fill_sample(text))):
            if part:
                words.append((part, f, color, size))
    lines, line, lw = [], [], 0
    for word in words:
        wl = word[1].getlength(word[0])
        if line and word[0].strip() and lw + wl > width:
            lines.append(line)
            line, lw = [], 0
            if not word[0].strip():
                continue
        line.append(word)
        lw += wl
    if line:
        lines.append(line)
    return lines


def draw_text(d, x, y, w, h, paragraphs, opts):
    blocks, total = [], 0
    for p in paragraphs:
        meta, runs = p[0], p[1:]
        indent = px(BULLET_INDENT) if meta.get("bullet") else 0
        lines = layout_paragraph(runs, px(w) - indent)
        for i, line in enumerate(lines):
            size = max(s for _, _, _, s in line)
            lh = size / 72 * DPI * 1.2 * meta["line"]
            blocks.append((line, lh, size, indent, meta.get("bullet") and i == 0))
            total += lh
        total += meta["space_after"] / 72 * DPI
        blocks.append((None, meta["space_after"] / 72 * DPI, 0, 0, False))
    anchor = opts.get("anchor", "t")
    cy = px(y) + {"t": 0, "m": (px(h) - total) / 2, "b": px(h) - total}[anchor]
    if total > px(h) + 2:
        d.rectangle([px(x), px(y), px(x + w), px(y + h)], outline=(220, 0, 0), width=3)  # overflow marker
    for line, lh, size, indent, dot in blocks:
        if line is None:
            cy += lh
            continue
        lw = sum(f.getlength(t) for t, f, _, _ in line)
        cx = px(x) + indent + {"l": 0, "c": (px(w) - lw) / 2, "r": px(w) - lw}[opts.get("align", "l")]
        base = cy + size / 72 * DPI
        if dot:
            r = size / 72 * DPI * 0.16
            cxd, cyd = px(x) + indent / 3, base - size / 72 * DPI * 0.3
            d.ellipse([cxd - r, cyd - r, cxd + r, cyd + r], fill=rgb(GOLD))
        for t, f, color, _ in line:
            d.text((cx, base), t, font=f, fill=rgb(color), anchor="ls")
            cx += f.getlength(t)
        cy += lh


def preview(directory):
    os.makedirs(directory, exist_ok=True)
    for n, elements in enumerate(SLIDES, 1):
        img = Image.new("RGB", (px(W), px(H)), rgb(WHITE))
        d = ImageDraw.Draw(img)
        for el in elements:
            kind = el[0]
            if kind == "rect":
                _, x, y, w, h, fill, opts = el
                d.rectangle([px(x), px(y), px(x + w) - 1, px(y + h) - 1], fill=rgb(fill), outline=rgb(opts["line"]) if opts.get("line") else None)
            elif kind == "text":
                draw_text(d, *el[1:6], el[6])
            elif kind == "image":
                _, x, y, w, h, path = el
                pic = Image.open(path).convert("RGBA").resize((px(w), px(h)))
                img.paste(pic, (px(x), px(y)), pic)
            elif kind == "photo":
                _, x, y, w, h, label = el
                pic = Image.open(placeholder_photo(label)).resize((px(w), px(h)))
                img.paste(pic, (px(x), px(y)))
        img.save(os.path.join(directory, f"slide{n}.png"))


if __name__ == "__main__":
    media = build_pptx()
    for n, names in enumerate(media, 1):
        if names:
            print(f"slide{n}: {names}")
    if "--preview" in sys.argv:
        preview(sys.argv[sys.argv.index("--preview") + 1])
