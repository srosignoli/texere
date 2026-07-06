import { writable } from 'svelte/store'

// studyOpen — true while the Recall (Study) overlay is mounted.
export const studyOpen = writable(false)
// studySurfaceOpen — the unified Study surface overlay is visible.
export const studySurfaceOpen = writable(false)
// studyFocus — 'next' | 'read' | 'process' | 'recall'
export const studyFocus = writable('next')
