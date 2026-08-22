package constraint

import (
	"testing"

	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// funcResolver wraps field/index functions as a Resolver for tests.
type funcResolver struct {
	field func(t typ.Type, name string) (typ.Type, bool)
	index func(t typ.Type, key typ.Type) (typ.Type, bool)
}

func (r *funcResolver) Field(t typ.Type, name string) (typ.Type, bool) {
	if r.field == nil {
		return nil, false
	}
	return r.field(t, name)
}

func (r *funcResolver) Index(t typ.Type, key typ.Type) (typ.Type, bool) {
	if r.index == nil {
		return nil, false
	}
	return r.index(t, key)
}

func TestTypesOverlap_DifferentInstantiations(t *testing.T) {
	// Create two different record types
	eventType := typ.NewRecord().Field("kind", typ.String).Build()
	timeType := typ.NewRecord().Field("sec", typ.Number).Build()

	// Create Channel<T> generic
	tParam := typ.NewTypeParam("T", nil)
	channelInterface := typ.NewInterface("Channel", []typ.Method{
		{Name: "receive", Type: typ.Func().Param("self", typ.Self).Returns(tParam).Build()},
	})
	channelGeneric := typ.NewGeneric("Channel", []*typ.TypeParam{tParam}, channelInterface)

	// Create instantiations
	chanEvent := typ.Instantiate(channelGeneric, eventType)
	chanTime := typ.Instantiate(channelGeneric, timeType)

	// They should NOT overlap because type args differ
	if narrow.TypesOverlap(chanEvent, chanTime) {
		t.Errorf("Channel<Event> and Channel<Time> should NOT overlap, but typesOverlap returned true")
	}

	// Same type should overlap
	if !narrow.TypesOverlap(chanEvent, chanEvent) {
		t.Errorf("Channel<Event> should overlap with itself")
	}
}

func TestFieldMatchesType_ChannelSelect(t *testing.T) {
	// Create two different record types
	eventType := typ.NewRecord().Field("kind", typ.String).Build()
	timeType := typ.NewRecord().Field("sec", typ.Number).Build()

	// Create Channel<T> generic
	tParam := typ.NewTypeParam("T", nil)
	channelInterface := typ.NewInterface("Channel", []typ.Method{
		{Name: "receive", Type: typ.Func().Param("self", typ.Self).Returns(tParam).Build()},
	})
	channelGeneric := typ.NewGeneric("Channel", []*typ.TypeParam{tParam}, channelInterface)

	// Create instantiations
	chanEvent := typ.Instantiate(channelGeneric, eventType)
	chanTime := typ.Instantiate(channelGeneric, timeType)

	// Build select result union
	variant1 := typ.NewRecord().
		Field("channel", chanEvent).
		Field("value", eventType).
		Field("ok", typ.Boolean).
		Build()
	variant2 := typ.NewRecord().
		Field("channel", chanTime).
		Field("value", timeType).
		Field("ok", typ.Boolean).
		Build()
	resultUnion := typ.NewUnion(variant1, variant2)

	// Resolver for field queries
	resolver := &funcResolver{
		field: func(t typ.Type, name string) (typ.Type, bool) {
			if rec, ok := t.(*typ.Record); ok {
				for _, f := range rec.Fields {
					if f.Name == name {
						return f.Type, true
					}
				}
			}
			return nil, false
		},
	}

	// Test narrowByFieldType: narrow to variant1 when channel == chanEvent
	narrowed := narrowByFieldType(resultUnion, "channel", chanEvent, resolver)
	if narrowed == nil {
		t.Fatal("narrowByFieldType returned nil")
	}
	if !typ.TypeEquals(narrowed, variant1) {
		t.Errorf("narrowByFieldType should narrow to variant1, got %v", narrowed)
	}

	// Test excludeByFieldType: exclude variant2 when channel == chanTime
	excluded := excludeByFieldType(resultUnion, "channel", chanTime, resolver)
	if excluded == nil {
		t.Fatal("excludeByFieldType returned nil")
	}
	if !typ.TypeEquals(excluded, variant1) {
		t.Errorf("excludeByFieldType should exclude variant2 leaving variant1, got %v", excluded)
	}
}
