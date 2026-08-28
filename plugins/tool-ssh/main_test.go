package main

import "testing"

func TestHasAuthMethod(t *testing.T) {
	cases := []struct {
		name   string
		pw, pk string
		want   bool
	}{
		{"no method", "", "", false},
		{"password only", "secret", "", true},
		{"private key only", "", "/path/key", true},
		{"both", "secret", "/path/key", true},
	}
	for _, c := range cases {
		if got := hasAuthMethod(c.pw, c.pk); got != c.want {
			t.Errorf("%s: hasAuthMethod(%q,%q) = %v, want %v", c.name, c.pw, c.pk, got, c.want)
		}
	}
}

func TestResolveSSHPort(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 22},
		{-1, 22},
		{2222, 2222},
	}
	for _, c := range cases {
		if got := resolveSSHPort(c.in); got != c.want {
			t.Errorf("resolveSSHPort(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
