package handler_test

import (
	"ezproxy/handler"
	"testing"
)

func TestFlagServerbound(t *testing.T) {
	v := handler.CapFlags(0)
	v |= handler.CapFlag_ToServer
	if !v.IsServerbound() {
		t.Errorf("expeted serverbound but IsServerbound is false value '%d'", v)
	}
	if v.IsClientbound() {
		t.Errorf("expeted serverbound but IsClientbound is true value '%d'", v)
	}
	if v.IsInjected() {
		t.Errorf("expeted serverbound but IsInjected is true value '%d'", v)
	}
}

func TestFlagClientbound(t *testing.T) {
	v := handler.CapFlags(0)
	if v.IsServerbound() {
		t.Errorf("expeted clientbound but IsServerbound is true value '%d'", v)
	}
	if !v.IsClientbound() {
		t.Errorf("expeted clientbound but IsClientbound is false value '%d'", v)
	}
	if v.IsInjected() {
		t.Errorf("expeted clientbound but IsInjected is true value '%d'", v)
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
