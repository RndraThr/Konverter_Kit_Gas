import { afterEach, describe, expect, it, vi } from 'vitest'
import { downscaleLogo, resizeImage } from './imageResize'

function mockCanvas() {
  const drawImage = vi.fn()
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage } as unknown as CanvasRenderingContext2D)
  const sizes: Array<[number, number]> = []
  const toBlob = vi
    .spyOn(HTMLCanvasElement.prototype, 'toBlob')
    .mockImplementation(function (this: HTMLCanvasElement, cb: BlobCallback, type?: string) {
      sizes.push([this.width, this.height])
      cb(new Blob(['x'], { type: type ?? 'image/png' }))
    })
  return { drawImage, toBlob, sizes }
}

function mockBitmap(width: number, height: number) {
  const close = vi.fn()
  vi.stubGlobal('createImageBitmap', vi.fn().mockResolvedValue({ width, height, close }))
  return close
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('resizeImage', () => {
  it('downsizes 4000x3000 to 1920x1440 at standar', async () => {
    const { sizes, toBlob, drawImage } = mockCanvas()
    const close = mockBitmap(4000, 3000)
    const file = await resizeImage(new Blob(['a']), 'standar')
    expect(sizes[0]).toEqual([1920, 1440])
    expect(toBlob).toHaveBeenCalledWith(expect.any(Function), 'image/jpeg', 0.92)
    expect(drawImage).toHaveBeenCalledWith(expect.anything(), 0, 0, 1920, 1440)
    expect(close).toHaveBeenCalled()
    expect(file.name).toBe('photo.jpg')
    expect(file.type).toBe('image/jpeg')
  })

  it('keeps full size at asli with quality 0.95', async () => {
    const { sizes, toBlob } = mockCanvas()
    mockBitmap(4000, 3000)
    await resizeImage(new Blob(['a']), 'asli')
    expect(sizes[0]).toEqual([4000, 3000])
    expect(toBlob).toHaveBeenCalledWith(expect.any(Function), 'image/jpeg', 0.95)
  })

  it('never upscales', async () => {
    const { sizes } = mockCanvas()
    mockBitmap(800, 600)
    await resizeImage(new Blob(['a']), 'hemat')
    expect(sizes[0]).toEqual([800, 600])
  })

  it('handles portrait by longest side', async () => {
    const { sizes } = mockCanvas()
    mockBitmap(3000, 4000)
    await resizeImage(new Blob(['a']), 'hemat')
    expect(sizes[0]).toEqual([960, 1280])
  })

  it('rejects when encoding fails', async () => {
    mockCanvas()
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((cb) => cb(null))
    mockBitmap(10, 10)
    await expect(resizeImage(new Blob(['a']), 'asli')).rejects.toThrow()
  })
})

describe('downscaleLogo', () => {
  it('caps the longest side at 512 and returns a PNG data URL', async () => {
    const { sizes } = mockCanvas()
    mockBitmap(1024, 256)
    vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation(function (this: HTMLCanvasElement, cb: BlobCallback, type?: string) {
      sizes.push([this.width, this.height])
      cb(new Blob(['png'], { type: type ?? 'image/png' }))
    })
    const url = await downscaleLogo(new Blob(['a']))
    expect(sizes.at(-1)).toEqual([512, 128])
    expect(url.startsWith('data:image/png')).toBe(true)
  })
})
