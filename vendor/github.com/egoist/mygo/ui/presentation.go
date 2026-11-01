package ui

// AfterPresent runs fn once on the UI thread after this build's frame has been
// submitted to the surface. Register during each build that needs a callback;
// earlier passes in the same frame are discarded. It does not mean scanout or
// that the user saw the pixels. Callbacks must not retain build elements.
func (c *Context) AfterPresent(fn func()) {
	if c.build() != nil && fn != nil {
		c.rt.afterPresent = append(c.rt.afterPresent, fn)
	}
}

func (rt *engine) notifyPresented() {
	if h, ok := rt.host.(*windowHost); ok && h.framePath() == "" {
		return
	}
	callbacks := rt.afterPresent
	rt.afterPresent = nil
	for _, fn := range callbacks {
		fn()
	}
}
