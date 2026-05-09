package dcol

import "fmt"

// Handler processes one validated DCOL frame after framing and checksum verification.
// Return (nil, nil) when no Message should be emitted yet (e.g. incomplete multi-page GSOF).
type Handler interface {
	Handle(p *Parser, fr Frame, env Env) (*Message, error)
}

// Registry maps DCOL packet type bytes (third byte of frame, index 2) to handlers.
//
// Private or non-public command implementations can live in another Go module that depends on
// this one and calls Registry.Register or RegisterOrReplace from an init function or main:
//
//	func init() {
//	    dcol.RegisterOrReplace(reg, 0xAB, myPrivateHandler{})
//	}
//
// That is compile-time linking via the Go module graph; there is no global registry in this package.
// For experimental runtime extension, build with Go's plugin mechanism (same Go toolchain/version)
// or shell out to a helper process — both are outside this package's scope.
type Registry struct {
	handlers map[byte]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[byte]Handler)}
}

// Register installs a handler for cmd. It errors if cmd is already registered.
func (r *Registry) Register(cmd byte, h Handler) error {
	if r.handlers == nil {
		r.handlers = make(map[byte]Handler)
	}
	if _, exists := r.handlers[cmd]; exists {
		return fmt.Errorf("dcol: duplicate handler for command 0x%02X", cmd)
	}
	r.handlers[cmd] = h
	return nil
}

// MustRegister calls Register and panics on duplicate registration.
func MustRegister(r *Registry, cmd byte, h Handler) {
	if err := r.Register(cmd, h); err != nil {
		panic(err)
	}
}

// RegisterOrReplace sets or replaces the handler for cmd (for private extensions overriding stubs).
func (r *Registry) RegisterOrReplace(cmd byte, h Handler) {
	if r.handlers == nil {
		r.handlers = make(map[byte]Handler)
	}
	r.handlers[cmd] = h
}

func (r *Registry) lookup(cmd byte) Handler {
	if r.handlers == nil {
		return stubHandler{}
	}
	if h, ok := r.handlers[cmd]; ok {
		return h
	}
	return stubHandler{}
}

// RegisterPublic registers the open-source Trimble-oriented handlers shipped with this module.
func RegisterPublic(r *Registry) {
	MustRegister(r, 0x40, handler40{})
	MustRegister(r, 0x93, handler93{})
	MustRegister(r, 0x98, handler98{})
}
