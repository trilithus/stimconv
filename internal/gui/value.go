package gui

// value is a piece of UI state. Setting it refreshes the window's dynamic
// widgets (see ui.relayout), so labels and buttons follow it. UI thread only.
type value[T comparable] struct {
	v T
	u *ui
}

func newValue[T comparable](u *ui, v T) *value[T] { return &value[T]{v: v, u: u} }

func (s *value[T]) Get() T { return s.v }

func (s *value[T]) Set(v T) {
	if s.v == v {
		return
	}
	s.v = v
	s.u.relayout()
}
