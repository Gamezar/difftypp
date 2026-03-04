import { describe, it, expect, beforeEach } from 'vitest'
import {
  serializeCursorHash,
  parseCursorHash,
  saveCursorToHash,
  restoreCursorFromHash,
} from './cursor-persistence'

describe('serializeCursorHash', () => {
  it('serializes a valid cursor position', () => {
    expect(serializeCursorHash(5, 'left')).toBe('cursor=5&side=left')
  })

  it('serializes index 0 (first line)', () => {
    expect(serializeCursorHash(0, 'right')).toBe('cursor=0&side=right')
  })

  it('defaults side to right when empty', () => {
    expect(serializeCursorHash(3, '')).toBe('cursor=3&side=right')
  })

  it('returns null for negative index', () => {
    expect(serializeCursorHash(-1, 'right')).toBeNull()
  })
})

describe('parseCursorHash', () => {
  it('parses a valid hash string', () => {
    expect(parseCursorHash('cursor=5&side=left')).toEqual({
      index: 5,
      side: 'left',
    })
  })

  it('parses hash with leading #', () => {
    expect(parseCursorHash('#cursor=10&side=right')).toEqual({
      index: 10,
      side: 'right',
    })
  })

  it('defaults side to right when missing', () => {
    expect(parseCursorHash('cursor=3')).toEqual({
      index: 3,
      side: 'right',
    })
  })

  it('returns null for empty hash', () => {
    expect(parseCursorHash('')).toBeNull()
    expect(parseCursorHash('#')).toBeNull()
  })

  it('returns null for non-numeric cursor', () => {
    expect(parseCursorHash('cursor=abc&side=left')).toBeNull()
  })

  it('returns null for negative cursor', () => {
    expect(parseCursorHash('cursor=-1&side=right')).toBeNull()
  })

  it('parses index 0 correctly', () => {
    expect(parseCursorHash('cursor=0&side=left')).toEqual({
      index: 0,
      side: 'left',
    })
  })
})

describe('saveCursorToHash', () => {
  beforeEach(() => {
    history.replaceState(null, '', window.location.pathname)
  })

  it('sets the hash with cursor position', () => {
    saveCursorToHash(7, 'left')
    expect(window.location.hash).toBe('#cursor=7&side=left')
  })

  it('does not set hash for negative index', () => {
    saveCursorToHash(-1, 'right')
    expect(window.location.hash).toBe('')
  })
})

describe('restoreCursorFromHash', () => {
  beforeEach(() => {
    history.replaceState(null, '', window.location.pathname)
  })

  it('restores cursor and cleans hash', () => {
    history.replaceState(null, '', '#cursor=4&side=left')
    const result = restoreCursorFromHash()
    expect(result).toEqual({ index: 4, side: 'left' })
    expect(window.location.hash).toBe('')
  })

  it('returns null when no hash present', () => {
    expect(restoreCursorFromHash()).toBeNull()
  })

  it('returns null for invalid hash and does not clean it', () => {
    history.replaceState(null, '', '#something-else')
    expect(restoreCursorFromHash()).toBeNull()
  })
})
