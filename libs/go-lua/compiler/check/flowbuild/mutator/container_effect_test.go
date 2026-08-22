package mutator_test

import (
	"testing"

	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestMethodLookup_Interface(t *testing.T) {
	iface := typ.NewInterface("test.Channel", []typ.Method{
		{
			Name: "send",
			Type: typ.Func().Param("self", typ.Self).Param("value", typ.Any).Returns(typ.Boolean).Build(),
		},
	})

	fnType, ok := core.Method(iface, "send")
	if !ok || fnType == nil {
		t.Fatalf("expected to find send method, got nil")
	}

	fn := unwrap.Function(fnType)
	if fn == nil {
		t.Fatalf("expected function type, got %T", fnType)
	}

	if len(fn.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(fn.Params))
	}
}

func TestMethodLookup_Instantiated(t *testing.T) {
	elemParam := typ.NewTypeParam("T", nil)

	sendSpec := contract.NewSpec().WithEffectRow(effect.Row{
		Labels: []effect.Label{
			effect.Mutate{
				Target: effect.ParamRef{Index: 0},
				Transform: effect.ContainerElementUnion{
					Container: effect.ParamRef{Index: 0},
					Value:     effect.ParamRef{Index: 1},
				},
			},
		},
	})

	channelType := typ.NewInterface("channel.Channel", []typ.Method{
		{
			Name: "send",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("value", elemParam).
				Returns(typ.Boolean).
				Spec(sendSpec).
				Build(),
		},
	})
	channelGeneric := typ.NewGeneric("channel.Channel", []*typ.TypeParam{elemParam}, channelType)

	inst := typ.Instantiate(channelGeneric, typ.Unknown)

	fnType, ok := core.Method(inst, "send")
	if !ok || fnType == nil {
		t.Fatalf("expected to find send method on instantiated type, got nil")
	}

	fn := unwrap.Function(fnType)
	if fn == nil {
		t.Fatalf("expected function type, got %T", fnType)
	}

	if fn.Spec == nil {
		t.Errorf("expected spec to be preserved, got nil")
	}

	spec, ok := fn.Spec.(*contract.Spec)
	if !ok {
		t.Errorf("expected *contract.Spec, got %T", fn.Spec)
	}

	found := false
	for _, label := range spec.Effects.Labels {
		if mut, ok := label.(effect.Mutate); ok {
			if _, ok := mut.Transform.(effect.ContainerElementUnion); ok {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("expected ContainerElementUnion effect in spec, got %v", spec.Effects)
	}
}

func TestSubstituteParams_PreservesSpec(t *testing.T) {
	elemParam := typ.NewTypeParam("T", nil)

	sendSpec := contract.NewSpec().WithEffectRow(effect.Row{
		Labels: []effect.Label{
			effect.Mutate{
				Target: effect.ParamRef{Index: 0},
				Transform: effect.ContainerElementUnion{
					Container: effect.ParamRef{Index: 0},
					Value:     effect.ParamRef{Index: 1},
				},
			},
		},
	})

	origFn := typ.Func().
		Param("self", typ.Self).
		Param("value", elemParam).
		Returns(typ.Boolean).
		Spec(sendSpec).
		Build()

	fn := unwrap.Function(origFn)
	if fn.Spec == nil {
		t.Fatal("original function has no spec")
	}

	resultType := subst.Params(fn, []*typ.TypeParam{elemParam}, []typ.Type{typ.Number})

	resultFn := unwrap.Function(resultType)
	if resultFn == nil {
		t.Fatal("result is not a function")
	}

	if resultFn.Spec == nil {
		t.Errorf("spec was not preserved during substitution")
	}
}

func TestContainerMutatorFromCall_DetectsEffect(t *testing.T) {
	elemParam := typ.NewTypeParam("T", nil)

	sendSpec := contract.NewSpec().WithEffectRow(effect.Row{
		Labels: []effect.Label{
			effect.Mutate{
				Target: effect.ParamRef{Index: 0},
				Transform: effect.ContainerElementUnion{
					Container: effect.ParamRef{Index: 0},
					Value:     effect.ParamRef{Index: 1},
				},
			},
		},
	})

	channelType := typ.NewInterface("channel.Channel", []typ.Method{
		{
			Name: "send",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("value", elemParam).
				Returns(typ.Boolean).
				Spec(sendSpec).
				Build(),
		},
	})
	channelGeneric := typ.NewGeneric("channel.Channel", []*typ.TypeParam{elemParam}, channelType)

	inst := typ.Instantiate(channelGeneric, typ.Unknown)

	fnType, ok := core.Method(inst, "send")
	if !ok || fnType == nil {
		t.Fatalf("expected to find send method")
	}

	fn := unwrap.Function(fnType)
	if fn == nil || fn.Spec == nil {
		t.Fatalf("expected function with spec")
	}

	spec, isSpec := fn.Spec.(*contract.Spec)
	if !isSpec {
		t.Fatalf("expected *contract.Spec")
	}

	var ceu *effect.ContainerElementUnion
	for _, label := range spec.Effects.Labels {
		mut, ok := label.(effect.Mutate)
		if !ok {
			continue
		}
		if u, ok := mut.Transform.(effect.ContainerElementUnion); ok {
			ceu = &u
			break
		}
	}

	if ceu == nil {
		t.Errorf("expected to extract ContainerElementUnion, got nil")
	} else {
		if ceu.Container.Index != 0 {
			t.Errorf("expected Container.Index = 0, got %d", ceu.Container.Index)
		}
		if ceu.Value.Index != 1 {
			t.Errorf("expected Value.Index = 1, got %d", ceu.Value.Index)
		}
	}
}
