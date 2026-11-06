// Dev-only PNG portraits, copied from builder-console/src/dev/mock-images.ts.
import { deflateSync } from 'node:zlib'

export type Rgb = readonly [number, number, number]

const CRC_TABLE = Array.from({ length: 256 }, (_, n) => {
  let c = n
  for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
  return c >>> 0
})

function crc32(bytes: Uint8Array): number {
  let c = 0xffffffff
  for (const byte of bytes) c = CRC_TABLE[(c ^ byte) & 0xff]! ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

function pngChunk(type: string, data: Uint8Array): Uint8Array {
  const chunk = new Uint8Array(12 + data.length)
  const view = new DataView(chunk.buffer)
  view.setUint32(0, data.length)
  for (let index = 0; index < 4; index++) chunk[4 + index] = type.charCodeAt(index)
  chunk.set(data, 8)
  view.setUint32(8 + data.length, crc32(chunk.subarray(4, 8 + data.length)))
  return chunk
}

const tint = ([r, g, b]: Rgb, amount: number): Rgb =>
  [Math.round(r + (255 - r) * amount), Math.round(g + (255 - g) * amount), Math.round(b + (255 - b) * amount)]

/** A real, decodable square PNG portrait: a head-and-shoulders silhouette in `color` on its pale tint. */
export function mockAvatarPng(color: Rgb, size = 96): Uint8Array<ArrayBuffer> {
  const stride = size * 3 + 1
  const raw = new Uint8Array(stride * size)
  const background = tint(color, 0.75)
  const head = { x: size / 2, y: size * 0.4, r: size * 0.2 }
  const body = { x: size / 2, y: size * 1.02, r: size * 0.38 }
  const inside = (x: number, y: number, circle: { x: number; y: number; r: number }) => (x - circle.x) ** 2 + (y - circle.y) ** 2 <= circle.r ** 2
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const px = x + 0.5
      const py = y + 0.5
      raw.set(inside(px, py, head) || inside(px, py, body) ? color : background, y * stride + 1 + x * 3)
    }
  }
  return encodePng(size, size, raw)
}

/** 8-bit RGB scanlines (each prefixed with filter byte 0) → PNG file bytes. */
function encodePng(width: number, height: number, raw: Uint8Array): Uint8Array<ArrayBuffer> {
  const header = new Uint8Array(13)
  const view = new DataView(header.buffer)
  view.setUint32(0, width)
  view.setUint32(4, height)
  header.set([8, 2, 0, 0, 0], 8) // 8-bit RGB, deflate, no filter, no interlace
  const parts = [
    new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk('IHDR', header),
    pngChunk('IDAT', new Uint8Array(deflateSync(raw))),
    pngChunk('IEND', new Uint8Array()),
  ]
  const png = new Uint8Array(parts.reduce((sum, part) => sum + part.length, 0))
  let offset = 0
  for (const part of parts) {
    png.set(part, offset)
    offset += part.length
  }
  return png
}
