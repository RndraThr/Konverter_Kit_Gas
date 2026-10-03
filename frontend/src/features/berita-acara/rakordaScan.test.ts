import { describe, expect, it } from 'vitest';
import { applyScanColor, buildJpegPdf } from './rakordaScan';

describe('RAKORDA scan processing', () => {
  it('enhances color while preserving chroma', () => {
    const pixels = new Uint8ClampedArray([90, 120, 150, 255, 240, 235, 230, 255]);
    applyScanColor(pixels, 2, 1, 'enhanced');
    expect(pixels[0]).not.toBe(pixels[1]);
    expect(pixels[3]).toBe(255);
    expect(pixels[4]).toBeGreaterThan(240);
  });

  it('binarizes dark text against a locally bright background even when half the page is shadowed', () => {
    const width = 40, height = 1;
    const pixels = new Uint8ClampedArray(width * height * 4);
    for (let x = 0; x < width; x++) {
      // Left half simulates a shadowed region (background ~90), right half
      // a brightly lit region (background ~220); a dark stroke (~20) sits
      // in each half. A single fixed threshold cannot binarize both.
      const shadowed = x < width / 2;
      const isStroke = x === 5 || x === 35;
      const value = isStroke ? 20 : shadowed ? 90 : 220;
      const index = x * 4;
      pixels[index] = pixels[index + 1] = pixels[index + 2] = value;
      pixels[index + 3] = 255;
    }
    applyScanColor(pixels, width, height, 'black-white');
    expect(pixels[5 * 4]).toBe(0);
    expect(pixels[35 * 4]).toBe(0);
    expect(pixels[2 * 4]).toBe(255);
    expect(pixels[38 * 4]).toBe(255);
  });

  it('creates a multi-page PDF from JPEG pages', () => {
    const pdf = buildJpegPdf([
      { bytes: new Uint8Array([0xff, 0xd8, 0xff, 0xd9]), width: 100, height: 200 },
      { bytes: new Uint8Array([0xff, 0xd8, 0xff, 0xd9]), width: 200, height: 100 },
    ]);
    const text = new TextDecoder().decode(pdf);
    expect(text.startsWith('%PDF-1.4')).toBe(true);
    expect(text).toContain('/Count 2');
    expect(text.match(/\/Type \/Page\b/g)).toHaveLength(2);
  });
});
