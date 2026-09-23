# Collection Session — design

Date: 2026-09-23 · Branch: `099-collection-session` · Vocabulary: `CONTEXT.md` (Collection, Session)

## Problem

The open Collection has no owner. `App` holds its parts as loose fields (`collectionDir`, `learnStore`,
`searchSvc`, `explorerSvc`, `watcher`, `openFilePath`, `savingPaths`) and every lifecycle re-derives them:

- open/close of search, explorer, learning store and watcher is written three times (`startup`,
  `OpenCollection`, `BuildGraph`), and the explorer re-point after a rebuild was already a restart-required bug;
- `a.learnStore == nil` is checked 34 times, "no collection open" is spelled 39 times, `a.collectionDir` is read raw 41 times;
- the "self-write + sync + `file:ids-injected`" block is copied verbatim in `SyncStudyItems` and `BuildGraph`;
- Wails runs each bound call on its own goroutine, yet `learnStore`, `searchSvc`, `openFilePath` and
  `watcher` are read and written with no lock; `consumeWatchEvents` re-reads `a.watcher` on every loop,
  so after a switch the old goroutine can read the new watcher's channel;
- `GetSourceBytes`, `GetSourceHTML`, `GetSourceProjection` and `assembleContext` join a stored
  `refPath` onto the directory without `collectionpath.Resolve`, so `../` escapes the Collection;
- root tests build `&App{…}` by hand 43 times, and no test can exercise reopen or switch.

## Decisions (agreed)

1. **The Session owns everything that lives only while a Collection is open:** directory, learning
   store, search, explorer, watcher and its goroutine, the self-write set, the open file. The arrow
   registry is app-level and is passed in, not owned. LLM streams, shell runs, config stay on App.
2. **Package `internal/collection`**, type `Session`. It must not import the Wails runtime.
3. **Opening:** the learning store is required (a failure fails `Open`); the watcher is optional
   (a failure is logged and the Session runs without live updates).
4. **Concurrency:** the Session holds its own `sync.RWMutex` guarding the lifecycle of its stores.
   Calls run under the read lock; `RebuildGraph` and `Close` take the write lock. App keeps only a
   small mutex around its pointer to the current Session. Streaming or network-bound calls copy what
   they need out and never hold the read lock while they stream, download or show a dialog.
5. **Switching:** open the new Session fully; if that fails the current one stays. Otherwise close the
   old one, then publish the new one (SSTorytime-KV graph state is process-wide, so an in-flight query
   on the old Session must finish before any query on the new one starts).
6. **`RebuildGraph` includes** closing search, building, re-pointing the explorer, and the shared
   sync step (self-write guard around the sync + `file:ids-injected` for the open file).
7. **Events out** through a `Notify func(name string, data any)` passed to `Open`, with the event
   names as constants in `collection`. `graph:build-started/complete` stay in App's `BuildGraph`.
8. **Access to stores** through a callback: `Session.Read(func(h Handles) error)`. Handles are valid
   only inside the callback. App wraps it as `withCollection` / `inCollection`, returning
   `collection.ErrNoCollection` when nothing is open.
9. **Self-write** is one bracketing call: `SelfWrite(path, fn) error`. No public begin/end.
10. **`Handles.Path(rel)`** built on `collectionpath.Resolve` is the only way to turn a stored
    collection-relative path into a file path.
11. **Tests use a real Session** on `t.TempDir()`: no options, no fake. A `newTestApp(t, dir, reg)`
    helper replaces the App literals.
12. **`Open` returns the chapter summaries**; App keeps `initialSums` and `startupError`.

## Out of scope

Moving import logic into its own module (architecture candidate 2); moving review/extract/promote
transactions and `Rel`/`Resolve` into `learning.Store` (candidate 5); the `loadConfig`
corrupt-file data loss (separate fix).
