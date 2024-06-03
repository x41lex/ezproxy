package handler

// Capture flags
type CapFlags uint32

const (
	CapFlag_ToServer CapFlags = 1 << 0 // Direction, if set its Serverbound, if not is ClientBound
	CapFlag_Injected CapFlags = 1 << 1 // Is injected
)

// Is this serverbound
func (c CapFlags) IsServerbound() bool {
	return c&CapFlag_ToServer != 0
}

// Is this clientbound
func (c CapFlags) IsClientbound() bool {
	return c&CapFlag_ToServer == 0
}

// Is this injected
func (c CapFlags) IsInjected() bool {
	return c&CapFlag_Injected != 0
}
