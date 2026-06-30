package handler

// DefaultRegistry implements handlers.Registry using the global Register map.
// Passed to runner.Run from cmd/swl and tests.
type DefaultRegistry struct{}

func (DefaultRegistry) Get(id string) (any, bool) {
	return Get(id)
}

// Reg is the production registry singleton.
var Reg DefaultRegistry
