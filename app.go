package main

import (
	"context"
)

// App is the main application struct. It holds the Wails runtime context and
// serves as the binding surface between the Go backend and the Svelte frontend.
type App struct {
	ctx context.Context
}

// NewApp creates a new App instance.
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved so that runtime
// methods (e.g. wails.runtime.EventsEmit) can be called from other methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// domReady is called after the front-end dom has been loaded.
func (a *App) domReady(_ context.Context) {}

// beforeClose is called when the application is about to quit, either by
// clicking the window close button or calling runtime.Quit. Returning true
// will cause the application to continue, false will continue shutdown as normal.
func (a *App) beforeClose(_ context.Context) bool {
	return false
}

// shutdown is called at application termination.
func (a *App) shutdown(_ context.Context) {}
