import { writable } from 'svelte/store'

// readingOpen — Sources panel overlay visible.
export const readingOpen = writable(false)
// activeReading — the reading element currently open in the ReadingView, or null.
export const activeReading = writable(/** @type {any} */ (null))
