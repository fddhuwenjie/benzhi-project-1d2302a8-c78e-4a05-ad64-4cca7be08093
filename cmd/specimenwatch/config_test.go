package main

import "testing"

func TestResolveAddress(t *testing.T) {
	t.Setenv("PORT", "19999")
	got, err := resolveAddress("")
	if err != nil || got != "127.0.0.1:19999" {
		t.Fatalf("got %q, %v", got, err)
	}
	got, err = resolveAddress("127.0.0.1:19082")
	if err != nil || got != "127.0.0.1:19082" {
		t.Fatalf("explicit got %q, %v", got, err)
	}
	if _, err := resolveAddress("0.0.0.0:19081"); err == nil {
		t.Fatal("non-loopback address accepted")
	}
}
