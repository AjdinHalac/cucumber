package cucumber

// Initer allows to init service during registration
type Initer interface {
	Init(app *App)
}
