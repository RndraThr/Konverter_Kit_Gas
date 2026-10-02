import { describe, expect, it } from 'vitest'
import { uppercaseBusinessText } from './text'

describe('uppercaseBusinessText', () => {
  it('uppercases Unicode text without trimming typing whitespace', () => {
    expect(uppercaseBusinessText('Siti Núraeni  ')).toBe('SITI NÚRAENI  ')
  })

  it('keeps punctuation and numbers intact', () => {
    expect(uppercaseBusinessText('spwp 80-30 / 3 inch')).toBe('SPWP 80-30 / 3 INCH')
  })
})
