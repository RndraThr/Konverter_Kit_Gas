import { expect, test } from 'vitest'
import { placeKey, similarLocation } from './HandoverLocationField'

test('place keys ignore case, spaces and punctuation like the server', () => {
  expect(placeKey(' Balai  Ds. Tempe ')).toBe('BALAI DS TEMPE')
})

test('suggests a close existing location before a new one is added', () => {
  const locations = [{ id: 'a', name: 'Balai Desa Tempe' }, { id: 'b', name: 'Lapangan Sabbangparu' }]
  expect(similarLocation('Balai Dsea Tempe', locations)?.id).toBe('a')
  expect(similarLocation('balai desa tempe', locations)).toBeUndefined()
  expect(similarLocation('Gudang Kecamatan', locations)).toBeUndefined()
})
