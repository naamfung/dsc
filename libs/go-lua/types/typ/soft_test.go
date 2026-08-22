package typ

import "testing"

func TestIsSoftAnnotationPolicy(t *testing.T) {
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"nil", nil, false},
		{"any", Any, true},
		{"unknown", Unknown, true},
		{"optional any", NewOptional(Any), true},
		{"array any", NewArray(Any), true},
		{"map value any", NewMap(String, Any), true},
		{"union all soft", NewUnion(Any, Unknown), true},
		{"union mixed", NewUnion(String, Number), false},
		{"record map any", NewRecord().MapComponent(Integer, Any).Build(), true},
		{"record", NewRecord().Field("id", String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsSoft(tt.t, SoftAnnotationPolicy); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestIsSoftPlaceholderPolicy(t *testing.T) {
	emptyRecord := NewRecord().Build()
	emptyMapRecord := NewRecord().MapComponent(String, Any).Build()
	entryRecord := NewRecord().Field("id", String).Build()

	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"empty record", emptyRecord, true},
		{"record with field", entryRecord, false},
		{"record map any", emptyMapRecord, true},
		{"array of soft", NewArray(Any), true},
		{"union all soft", NewUnion(Any, emptyRecord), true},
		{"union mixed", NewUnion(emptyRecord, entryRecord), false},
	}

	for _, tt := range tests {
		if got := IsSoft(tt.t, SoftPlaceholderPolicy); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers(t *testing.T) {
	entryRecord := NewRecord().Field("id", String).Build()
	entryArray := NewArray(entryRecord)
	softArray := NewArray(Any)
	emptyRecord := NewRecord().Build()

	tests := []struct {
		name string
		in   Type
		want Type
	}{
		{"drop soft array", NewUnion(softArray, entryArray), entryArray},
		{"drop empty record", NewUnion(emptyRecord, entryArray), entryArray},
		{"all soft stays", NewUnion(Any, softArray), Any},
	}

	for _, tt := range tests {
		got := PruneSoftUnionMembers(tt.in)
		if !TypeEquals(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestPruneSoftUnionMembers_ReusesRewrittenSharedSubtrees(t *testing.T) {
	leaf := NewRecord().Field("id", String).Build()
	shared := NewRecord().Field("payload", NewUnion(NewRecord().Build(), leaf)).Build()
	root := NewRecord().Field("a", shared).Field("b", shared).Build()

	got := PruneSoftUnionMembers(root)
	rec, ok := got.(*Record)
	if !ok {
		t.Fatalf("expected record, got %T", got)
	}

	a := rec.GetField("a")
	b := rec.GetField("b")
	if a == nil || b == nil {
		t.Fatalf("expected fields a and b, got a=%v b=%v", a, b)
	}
	if a.Type != b.Type {
		t.Fatalf("expected rewritten shared subtree to be reused, got distinct pointers: %p vs %p", a.Type, b.Type)
	}

	sharedRec, ok := a.Type.(*Record)
	if !ok {
		t.Fatalf("expected rewritten shared record, got %T", a.Type)
	}
	payload := sharedRec.GetField("payload")
	if payload == nil || !TypeEquals(payload.Type, leaf) {
		t.Fatalf("expected payload field to prune to leaf record, got %v", payload)
	}
}

func TestPruneSoftUnionMembers_PrimitiveFastPath(t *testing.T) {
	got := PruneSoftUnionMembers(Number)
	if got != Number {
		t.Fatalf("expected primitive prune fast-path to return original singleton")
	}
}

func TestPruneSoftUnionMembers_AliasStillDescends(t *testing.T) {
	leaf := NewRecord().Field("id", String).Build()
	alias := NewAlias("T", NewUnion(NewRecord().Build(), leaf))
	got := PruneSoftUnionMembers(alias)
	gotAlias, ok := got.(*Alias)
	if !ok {
		t.Fatalf("expected alias result, got %T", got)
	}
	if !TypeEquals(gotAlias.Target, leaf) {
		t.Fatalf("expected alias target to be pruned to %v, got %v", leaf, gotAlias.Target)
	}
}

func TestIsRefinableAnnotation(t *testing.T) {
	tests := []struct {
		name string
		t    Type
		want bool
	}{
		{"nil", nil, false},
		{"any", Any, false},
		{"unknown", Unknown, false},
		{"optional any", NewOptional(Any), false},
		{"array any", NewArray(Any), true},
		{"record map any", NewRecord().MapComponent(String, Any).Build(), true},
		{"record", NewRecord().Field("id", String).Build(), false},
	}

	for _, tt := range tests {
		if got := IsRefinableAnnotation(tt.t); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
