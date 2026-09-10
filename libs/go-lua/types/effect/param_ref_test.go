package effect

import "testing"

func TestResolveParamIndex(t *testing.T) {
	tests := []struct {
		name     string
		ref      ParamRef
		argCount int
		wantIdx  int
		wantOK   bool
	}{
		{name: "absolute first", ref: ParamRef{Index: 0}, argCount: 2, wantIdx: 0, wantOK: true},
		{name: "absolute second", ref: ParamRef{Index: 1}, argCount: 2, wantIdx: 1, wantOK: true},
		{name: "tail last", ref: ParamRef{Index: -1}, argCount: 2, wantIdx: 1, wantOK: true},
		{name: "tail second last", ref: ParamRef{Index: -2}, argCount: 2, wantIdx: 0, wantOK: true},
		{name: "absolute out of range", ref: ParamRef{Index: 2}, argCount: 2, wantOK: false},
		{name: "tail out of range", ref: ParamRef{Index: -3}, argCount: 2, wantOK: false},
		{name: "empty args absolute", ref: ParamRef{Index: 0}, argCount: 0, wantOK: false},
		{name: "empty args tail", ref: ParamRef{Index: -1}, argCount: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIdx, gotOK := ResolveParamIndex(tt.ref, tt.argCount)
			if gotOK != tt.wantOK {
				t.Fatalf("ResolveParamIndex(%v, %d) ok = %v, want %v", tt.ref, tt.argCount, gotOK, tt.wantOK)
			}
			if gotOK && gotIdx != tt.wantIdx {
				t.Fatalf("ResolveParamIndex(%v, %d) idx = %d, want %d", tt.ref, tt.argCount, gotIdx, tt.wantIdx)
			}
		})
	}
}
