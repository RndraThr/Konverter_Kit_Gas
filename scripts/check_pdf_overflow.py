"""Report text that crosses a table border or leaves the page in rendered BA PDFs.

Usage: python scripts/check_pdf_overflow.py DIR_OR_PDF [...]

Pairs with TestStressRenderLongTextInEveryBA (internal/bast/render_overflow_test.go), which
writes long-text renders of every BA to BAST_STRESS_DIR. For every glyph the script checks
whether a ruled line (table border, cell divider) runs through the glyph's core, and whether the
glyph sits outside the printable page. Exit status 1 when anything overflows.
"""
import pathlib
import sys

import pymupdf

PAGE_MARGIN_PT = 4.0
# A glyph box includes ascender and descender space; only its core counts as "crossed".
CORE_X = 0.30
CORE_Y = 0.28


def segments(page):
    result = []
    for drawing in page.get_drawings():
        for item in drawing["items"]:
            if item[0] == "l":
                result.append((item[1].x, item[1].y, item[2].x, item[2].y))
            elif item[0] == "re":
                r = item[1]
                result += [(r.x0, r.y0, r.x1, r.y0), (r.x0, r.y1, r.x1, r.y1), (r.x0, r.y0, r.x0, r.y1), (r.x1, r.y0, r.x1, r.y1)]
    return result


def crosses(char_box, segment):
    x0, y0, x1, y1 = char_box
    w, h = x1 - x0, y1 - y0
    cx0, cx1 = x0 + w * CORE_X, x1 - w * CORE_X
    cy0, cy1 = y0 + h * CORE_Y, y1 - h * CORE_Y
    sx0, sy0, sx1, sy1 = segment
    if abs(sx0 - sx1) < 0.01:  # vertical line
        return cx0 < sx0 < cx1 and min(sy0, sy1) < cy1 and max(sy0, sy1) > cy0
    if abs(sy0 - sy1) < 0.01:  # horizontal line
        return cy0 < sy0 < cy1 and min(sx0, sx1) < cx1 and max(sx0, sx1) > cx0
    return False


def check(path):
    problems = []
    document = pymupdf.open(path)
    for page_number, page in enumerate(document, start=1):
        lines = segments(page)
        width, height = page.rect.width, page.rect.height
        for block in page.get_text("rawdict")["blocks"]:
            for line in block.get("lines", []):
                for span in line["spans"]:
                    text = "".join(c["c"] for c in span["chars"])
                    for char in span["chars"]:
                        if char["c"].isspace():
                            continue
                        box = char["bbox"]
                        reason = None
                        if box[3] > height - PAGE_MARGIN_PT / 2:
                            reason = "keluar halaman (bawah)"
                        elif box[0] < PAGE_MARGIN_PT or box[2] > width - PAGE_MARGIN_PT:
                            reason = "keluar halaman (samping)"
                        elif any(crosses(box, segment) for segment in lines):
                            reason = "memotong garis"
                        if reason:
                            problems.append((page_number, reason, text.strip()))
                            break
    return problems


def main(arguments):
    files = []
    for argument in arguments:
        path = pathlib.Path(argument)
        files += sorted(path.glob("*.pdf")) if path.is_dir() else [path]
    failed = False
    for file in files:
        problems = check(file)
        if not problems:
            print(f"OK    {file.name}")
            continue
        failed = True
        print(f"FAIL  {file.name}")
        seen = set()
        for page, reason, text in problems:
            key = (page, reason, text)
            if key in seen:
                continue
            seen.add(key)
            print(f"      hal {page} {reason}: {text[:90]}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
