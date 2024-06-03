package handler_test

import (
	"ezproxy/handler"
	"testing"
)

func TestFlagServerbound(t *testing.T) {
	v := handler.CapFlags(0)
	v |= handler.CapFlag_ToServer
	if !v.IsServerbound() {
		t.Errorf("expected serverbound but IsServerbound is false value '%d'", v)
	}
	if v.IsClientbound() {
		t.Errorf("expected serverbound but IsClientbound is true value '%d'", v)
	}
	if v.IsInjected() {
		t.Errorf("expected serverbound but IsInjected is true value '%d'", v)
	}
}

func TestFlagClientbound(t *testing.T) {
	v := handler.CapFlags(0)
	if v.IsServerbound() {
		t.Errorf("expected clientbound but IsServerbound is true value '%d'", v)
	}
	if !v.IsClientbound() {
		t.Errorf("expected clientbound but IsClientbound is false value '%d'", v)
	}
	if v.IsInjected() {
		t.Errorf("expected clientbound but IsInjected is true value '%d'", v)
	}
}

func TestFlagInjected(t *testing.T) {
	v := handler.CapFlags(0)
	if v.IsInjected() {
		t.Errorf("expected not injected but IsInjected is true value '%d'", v)
	}
	v |= handler.CapFlag_Injected
	if !v.IsInjected() {
		t.Errorf("expected injected but IsInjected is false value '%d'", v)
	}
}
