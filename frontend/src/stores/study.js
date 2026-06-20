import { writable } from 'svelte/store'

// studyOpen — true while the Recall (Study) overlay is mounted.
export const studyOpen = writable(false)
