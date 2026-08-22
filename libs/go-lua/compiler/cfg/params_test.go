package cfg

import "testing"

func TestParamSlot_HasSourceParam(t *testing.T) {
	tests := []struct {
		name string
		slot ParamSlot
		want bool
	}{
		{name: "source index zero", slot: ParamSlot{SourceIndex: 0}, want: true},
		{name: "source index positive", slot: ParamSlot{SourceIndex: 2}, want: true},
		{name: "implicit slot", slot: ParamSlot{SourceIndex: -1}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.slot.HasSourceParam(); got != tt.want {
				t.Fatalf("HasSourceParam() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParamSlot_SourceParamIndex(t *testing.T) {
	tests := []struct {
		name    string
		slot    ParamSlot
		wantIdx int
		wantOK  bool
	}{
		{name: "source index zero", slot: ParamSlot{SourceIndex: 0}, wantIdx: 0, wantOK: true},
		{name: "source index one", slot: ParamSlot{SourceIndex: 1}, wantIdx: 1, wantOK: true},
		{name: "implicit slot", slot: ParamSlot{SourceIndex: -1}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIdx, gotOK := tt.slot.SourceParamIndex()
			if gotOK != tt.wantOK {
				t.Fatalf("SourceParamIndex() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK && gotIdx != tt.wantIdx {
				t.Fatalf("SourceParamIndex() idx = %d, want %d", gotIdx, tt.wantIdx)
			}
		})
	}
}
