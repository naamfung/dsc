package typ

import "testing"

func TestCaptureModeString(t *testing.T) {
	cases := []struct {
		mode CaptureMode
		want string
	}{
		{CaptureUnknown, "unknown"},
		{CaptureByValue, "by-value"},
		{CaptureByRef, "by-ref"},
	}

	for _, tc := range cases {
		if got := tc.mode.String(); got != tc.want {
			t.Errorf("CaptureMode(%d).String() = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

func TestCaptureInfoNeedsUpvalue(t *testing.T) {
	cases := []struct {
		info CaptureInfo
		want bool
	}{
		{CaptureInfo{Mode: CaptureByValue}, false},
		{CaptureInfo{Mode: CaptureByRef}, true},
		{CaptureInfo{Mode: CaptureUnknown}, true},
		{CaptureInfo{Mode: CaptureByValue, Mutated: true}, true},
		{CaptureInfo{Mode: CaptureByValue, Escapes: true}, true},
	}

	for i, tc := range cases {
		if got := tc.info.NeedsUpvalue(); got != tc.want {
			t.Errorf("case %d: NeedsUpvalue() = %v, want %v", i, got, tc.want)
		}
	}
}
