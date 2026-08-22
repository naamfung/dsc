package inference

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestParamInfer_CallableParam tests inferring callable parameters.
func TestParamInfer_CallableParam(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "infer param is callable from call",
			Code: `
				function test(fn)
					local a, b = fn()
					return a, b
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "infer param callable with multi-value",
			Code: `
				function with_retry(fn, max_retries)
					local result, err = fn()
					if not err then
						return result
					end
					return nil, err
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestParamInfer_MethodAccess tests inferring parameters from method access.
func TestParamInfer_MethodAccess(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "infer param has method from method call",
			Code: `
				function handle_error(err)
					local k = err:kind()
					return k
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "infer param has multiple methods",
			Code: `
				function process(obj)
					local a = obj:first()
					local b = obj:second()
					return a, b
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestParamInfer_FieldAccess tests inferring parameters from field access.
func TestParamInfer_FieldAccess(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "infer param has field from field access",
			Code: `
				function get_name(user)
					return user.name
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "infer param has multiple fields",
			Code: `
				function get_info(user)
					return user.name, user.age
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestParamInfer_CallSiteInference tests inference from call sites.
func TestParamInfer_CallSiteInference(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "infer param type from call site",
			Code: `
				function with_retry(fn, max_retries)
					local result, err = fn()
					return result, err
				end

				local function flaky()
					return "success", nil
				end

				local r, e = with_retry(flaky, 3)
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestParamInfer_WithRetryPattern tests the common with_retry pattern.
func TestParamInfer_WithRetryPattern(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "with_retry pattern",
			Code: `
				local function with_retry(fn, max_retries)
					local result, err
					for _ = 1, max_retries do
						result, err = fn()
						if not err then
							return result
						end
					end
					return nil, err
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

// TestParamInfer_MultiValueExpansion tests multi-value return expansion.
func TestParamInfer_MultiValueExpansion(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "multi-value expansion from unknown",
			Code: `
				function test(fn)
					local a, b, c = fn()
					return a, b, c
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}
