import { describe, it, expect, vi, beforeAll } from 'vitest'

// ── Shared mock state ────────────────────────────────────────────

const callOrder: string[] = []

let capturedGetCursorState: (() => { index: number; side: string }) | null =
  null
let getterResultDuringPhase1: { index: number; side: string } | null = null
let capturedCommentApi: unknown = null

const mockCommentApi = {
  showCommentForm: vi.fn(),
  hideCommentForm: vi.fn(),
  clearLineSelection: vi.fn(),
  highlightRange: vi.fn(),
  isCommentFormVisible: vi.fn().mockReturnValue(false),
}

const mockCursorState = {
  index: 5,
  side: 'left',
  selectionAnchor: -1,
  prevIndex: -1,
  deleteConfirming: false,
  deleteTarget: null,
  pendingG: false,
  pendingGTimer: null,
}

// ── Mocks ────────────────────────────────────────────────────────

vi.mock('./comment-system', () => ({
  initializeCommentSystem: (
    getCursorState: () => { index: number; side: string }
  ) => {
    callOrder.push('commentSystem')
    // Capture the getter and call it NOW — before phase 2 sets cursorState
    capturedGetCursorState = getCursorState
    getterResultDuringPhase1 = getCursorState()
    return mockCommentApi
  },
}))

vi.mock('./cursor-navigation', () => ({
  initializeCursorNavigation: (commentApi: unknown) => {
    callOrder.push('cursorNavigation')
    capturedCommentApi = commentApi
    return mockCursorState
  },
}))

const mockInitStatusFilter = vi.fn(() => {
  callOrder.push('statusFilter')
})

vi.mock('./status-filter', () => ({
  initializeStatusFilter: mockInitStatusFilter,
}))

// ── Tests ────────────────────────────────────────────────────────

describe('diff/index entry point', () => {
  beforeAll(async () => {
    await import('./index')
    document.dispatchEvent(new Event('DOMContentLoaded'))
  })

  it('initializes comment system before cursor navigation', () => {
    expect(callOrder[0]).toBe('commentSystem')
    expect(callOrder[1]).toBe('cursorNavigation')
  })

  it('initializes status filter last', () => {
    expect(callOrder[2]).toBe('statusFilter')
  })

  it('passes comment API from phase 1 to cursor navigation', () => {
    expect(capturedCommentApi).toBe(mockCommentApi)
  })

  it('getCursorState returns default before phase 2', () => {
    expect(getterResultDuringPhase1).toEqual({ index: -1, side: 'right' })
  })

  it('getCursorState returns live cursor state after phase 2', () => {
    const state = capturedGetCursorState!()
    expect(state.index).toBe(5)
    expect(state.side).toBe('left')
  })

  it('getCursorState tracks mutations to cursor state object', () => {
    mockCursorState.index = 42
    mockCursorState.side = 'right'
    const state = capturedGetCursorState!()
    expect(state.index).toBe(42)
    expect(state.side).toBe('right')
    // Restore for other tests
    mockCursorState.index = 5
    mockCursorState.side = 'left'
  })
})
