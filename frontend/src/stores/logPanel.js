import { writable } from 'svelte/store'

/**
 * LogEntry shape:
 *   {
 *     id: number,
 *     severity: 'error' | 'warning' | 'info',
 *     source:   'parse' | 'build' | 'search',
 *     filePath?: string,    // absolute path, when available
 *     fileName?: string,    // basename, for compact display
 *     line?:     number,    // 1-indexed source line, when available
 *     message:   string,
 *     timestamp: number,    // Date.now() at append time
 *   }
 *
 * Diagnostic, session-scoped — no persistence. Cleared on app restart.
 */

export const logEntries = writable([])
export const logPanelOpen = writable(false)

let _nextId = 1

/**
 * Append one or more partial entries; id and timestamp are filled in.
 * Caller passes minimal shape (severity, source, message + optional file/line).
 */
export function appendLog(entries) {
  if (!entries || entries.length === 0) return
  const stamped = entries.map((e) => ({
    id: _nextId++,
    timestamp: Date.now(),
    ...e,
  }))
  logEntries.update((prev) => [...prev, ...stamped])
}

export function clearLog() {
  logEntries.set([])
}

export function toggleLogPanel() {
  logPanelOpen.update((v) => !v)
}
