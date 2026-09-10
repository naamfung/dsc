package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// createTimeManifest creates a manifest simulating the time module with Self-referential methods.
func createTimeManifest() *io.Manifest {
	m := io.NewManifest("time")

	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})

	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("t", typ.Self).Returns(durationType).Build()},
		{Name: "add", Type: typ.Func().Param("self", typ.Self).Param("d", durationType).Returns(typ.Self).Build()},
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})

	m.DefineType("Time", timeType)
	m.DefineType("Duration", durationType)

	moduleType := typ.NewInterface("time", []typ.Method{
		{Name: "now", Type: typ.Func().Returns(timeType).Build()},
	})
	m.SetExport(moduleType)

	return m
}

// createHTMLManifest creates a manifest simulating html module with chained method returns.
func createHTMLManifest() *io.Manifest {
	m := io.NewManifest("html")

	attrBuilderType := typ.NewInterface("html.AttrBuilder", []typ.Method{
		{Name: "on_elements", Type: typ.Func().Param("self", typ.Self).Variadic(typ.String).Returns(typ.Self).Build()},
		{Name: "globally", Type: typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()},
		{Name: "matching", Type: typ.Func().Param("self", typ.Self).Param("pattern", typ.String).Returns(typ.Self, typ.NewOptional(typ.LuaError)).Build()},
	})

	policyType := typ.NewInterface("html.Policy", []typ.Method{
		{Name: "allow_attrs", Type: typ.Func().Param("self", typ.Self).Variadic(typ.String).Returns(attrBuilderType).Build()},
		{Name: "sanitize", Type: typ.Func().Param("self", typ.Self).Param("html", typ.String).Returns(typ.String).Build()},
	})

	m.DefineType("Policy", policyType)
	m.DefineType("AttrBuilder", attrBuilderType)

	sanitizeType := typ.NewInterface("html.sanitize", []typ.Method{
		{Name: "new_policy", Type: typ.Func().Returns(policyType, typ.NewOptional(typ.LuaError)).Build()},
	})

	moduleType := typ.NewRecord().
		Field("sanitize", sanitizeType).
		Build()
	m.SetExport(moduleType)

	return m
}

// createHTTPClientManifest creates a manifest simulating http_client module.
func createHTTPClientManifest() *io.Manifest {
	m := io.NewManifest("http_client")

	responseType := typ.NewRecord().
		Field("status_code", typ.Number).
		Field("body", typ.String).
		Build()

	requestOptionsType := typ.NewRecord().
		OptField("headers", typ.NewMap(typ.String, typ.String)).
		OptField("body", typ.String).
		OptField("files", typ.NewArray(typ.Any)).
		Build()

	m.DefineType("Response", responseType)
	m.DefineType("RequestOptions", requestOptionsType)

	moduleType := typ.NewInterface("http_client", []typ.Method{
		{Name: "get", Type: typ.Func().Param("url", typ.String).OptParam("opts", requestOptionsType).Returns(responseType, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "post", Type: typ.Func().Param("url", typ.String).OptParam("opts", requestOptionsType).Returns(responseType, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "request", Type: typ.Func().Param("method", typ.String).Param("url", typ.String).OptParam("opts", requestOptionsType).Returns(responseType, typ.NewOptional(typ.LuaError)).Build()},
	})
	m.SetExport(moduleType)

	return m
}

// createStreamManifest creates a manifest simulating the stream module.
func createStreamManifest() *io.Manifest {
	m := io.NewManifest("stream")

	streamType := typ.NewInterface("stream.Stream", []typ.Method{
		{Name: "read", Type: typ.Func().Param("self", typ.Self).OptParam("n", typ.Number).Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "read_all", Type: typ.Func().Param("self", typ.Self).Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "write", Type: typ.Func().Param("self", typ.Self).Param("data", typ.String).Returns(typ.Number, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "close", Type: typ.Func().Param("self", typ.Self).Returns(typ.Boolean, typ.NewOptional(typ.LuaError)).Build()},
	})

	m.DefineType("Stream", streamType)
	moduleType := typ.NewInterface("stream", []typ.Method{})
	m.SetExport(moduleType)

	return m
}

// createHTTPWithStreamManifest creates an http manifest where request:stream() returns stream.Stream.
func createHTTPWithStreamManifest(streamType typ.Type) *io.Manifest {
	m := io.NewManifest("http")

	requestType := typ.NewInterface("http.Request", []typ.Method{
		{Name: "method", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "path", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
		{Name: "stream", Type: typ.Func().Param("self", typ.Self).Returns(streamType, typ.NewOptional(typ.LuaError)).Build()},
	})

	responseType := typ.NewInterface("http.Response", []typ.Method{
		{Name: "set_status", Type: typ.Func().Param("self", typ.Self).Param("status", typ.Number).Build()},
		{Name: "write", Type: typ.Func().Param("self", typ.Self).Param("data", typ.String).Build()},
	})

	m.DefineType("Request", requestType)
	m.DefineType("Response", responseType)
	m.DefineType("Stream", streamType)

	moduleType := typ.NewInterface("http", []typ.Method{
		{Name: "request", Type: typ.Func().OptParam("config", typ.Any).Returns(requestType, typ.NewOptional(typ.LuaError)).Build()},
		{Name: "response", Type: typ.Func().Returns(responseType, typ.NewOptional(typ.LuaError)).Build()},
	})
	m.SetExport(moduleType)

	return m
}

func TestModuleTypes_SelfTypeInMethodArguments(t *testing.T) {
	timeManifest := createTimeManifest()
	tests := []testutil.Case{
		{
			Name: "time.sub with another time value",
			Code: `
				local time = require("time")
				local start = time.now()
				local now = time.now()
				local elapsed = now:sub(start)
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"time": timeManifest},
		},
		{
			Name: "time.sub chained",
			Code: `
				local time = require("time")
				local start = time.now()
				local elapsed = time.now():sub(start)
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"time": timeManifest},
		},
		{
			Name: "time.add returns Self usable for sub",
			Code: `
				local time = require("time")
				local t = time.now()
				local duration = t:sub(t)
				local later = t:add(duration)
				local diff = later:sub(t)
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"time": timeManifest},
		},
	}
	testutil.RunCases(t, tests)
}

func TestModuleTypes_MethodChainingReturnTypes(t *testing.T) {
	htmlManifest := createHTMLManifest()
	tests := []testutil.Case{
		{
			Name: "policy allow_attrs returns AttrBuilder with globally",
			Code: `
				local html = require("html")
				local policy, err = html.sanitize.new_policy()
				if err then return end
				policy:allow_attrs("class"):globally()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"html": htmlManifest},
		},
		{
			Name: "AttrBuilder on_elements chains to globally",
			Code: `
				local html = require("html")
				local policy, err = html.sanitize.new_policy()
				if err then return end
				policy:allow_attrs("href"):on_elements("a"):globally()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"html": htmlManifest},
		},
		{
			Name: "matching returns Self with optional error",
			Code: `
				local html = require("html")
				local policy, err = html.sanitize.new_policy()
				if err then return end
				local builder, err2 = policy:allow_attrs("href"):matching("^https://")
				if err2 then return end
				builder:globally()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"html": htmlManifest},
		},
	}
	testutil.RunCases(t, tests)
}

func TestModuleTypes_ModuleMethodResolution(t *testing.T) {
	httpClientManifest := createHTTPClientManifest()
	tests := []testutil.Case{
		{
			Name: "request with method url and options",
			Code: `
				local http = require("http_client")
				local resp, err = http.request("OPTIONS", "http://localhost/test", {
					headers = {["Content-Type"] = "application/json"}
				})
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"http_client": httpClientManifest},
		},
		{
			Name: "request with method and url only",
			Code: `
				local http = require("http_client")
				local resp, err = http.request("GET", "http://localhost/test")
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"http_client": httpClientManifest},
		},
		{
			Name: "get with url only",
			Code: `
				local http = require("http_client")
				local resp, err = http.get("http://localhost/test")
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"http_client": httpClientManifest},
		},
		{
			Name: "post with url and options",
			Code: `
				local http = require("http_client")
				local resp, err = http.post("http://localhost/test", {body = "data"})
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"http_client": httpClientManifest},
		},
	}
	testutil.RunCases(t, tests)
}

func TestModuleTypes_IntegerSubtypeOfNumber(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "integer assignable to number",
			Code: `
				local x: integer = 42
				local y: number = x
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer literal assignable to number",
			Code: `
				local x: number = 42
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "record with integer assignable to record with number",
			Code: `
				local function pair(k: string, v: integer): {key: string, value: integer}
					return {key = k, value = v}
				end
				local p: {key: string, value: number} = pair("age", 30)
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "integer array assignable to number array",
			Code: `
				local arr: {integer} = {1, 2, 3}
				local nums: {number} = arr
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "function returning integer usable where number expected",
			Code: `
				local function getInt(): integer return 5 end
				local function useNum(n: number) return n * 2 end
				local result = useNum(getInt())
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestModuleTypes_ArraySubtyping(t *testing.T) {
	httpClientManifest := createHTTPClientManifest()
	tests := []testutil.Case{
		{
			Name: "specific record array assignable to any array",
			Code: `
				local files: {any} = {
					{name = "file1", content = "data1"},
					{name = "file2", content = "data2"}
				}
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "files field accepts specific record array",
			Code: `
				local http = require("http_client")
				local resp, err = http.post("http://localhost/upload", {
					files = {
						{name = "file", filename = "test.txt", content = "hello", content_type = "text/plain"}
					}
				})
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: map[string]*io.Manifest{"http_client": httpClientManifest},
		},
		{
			Name: "string array assignable to any array",
			Code: `
				local strs: {string} = {"a", "b", "c"}
				local anys: {any} = strs
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "number array assignable to any array",
			Code: `
				local nums: {number} = {1, 2, 3}
				local anys: {any} = nums
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestModuleTypes_SelectReturnType(t *testing.T) {
	tests := []testutil.Case{
		{
			Name: "select count returns number",
			Code: `
				local function count_args(...: any): number
					return select("#", ...)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "select count usable in arithmetic",
			Code: `
				local function f(...: any)
					local n = select("#", ...) + 1
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
		{
			Name: "select with index returns element",
			Code: `
				local function first(...: any): any
					return select(1, ...)
				end
			`,
			WantError: false,
			Stdlib:    true,
		},
	}
	testutil.RunCases(t, tests)
}

func TestModuleTypes_StreamTypeShadowing(t *testing.T) {
	streamManifest := createStreamManifest()
	streamType, _ := streamManifest.LookupType("Stream")
	httpManifest := createHTTPWithStreamManifest(streamType)
	manifests := map[string]*io.Manifest{
		"stream": streamManifest,
		"http":   httpManifest,
	}
	tests := []testutil.Case{
		{
			Name: "stream variable from req:stream() has correct type",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				local data = stream:read(1024)
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
		{
			Name: "stream:close() works after error check",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				stream:close()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
		{
			Name: "stream in while loop has correct type",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				while true do
					local chunk = stream:read(1024)
					if chunk == nil then break end
				end
				stream:close()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
		{
			Name: "stream without loop should work",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				local chunk1 = stream:read(1024)
				local chunk2 = stream:read(1024)
				stream:close()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
		{
			Name: "stream with if-else join",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				if true then
					local a = stream:read(1024)
				else
					local b = stream:read(1024)
				end
				stream:close()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
		{
			Name: "stream with for loop",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				for i = 1, 10 do
					local chunk = stream:read(1024)
				end
				stream:close()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
		{
			Name: "stream with while var loop",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				local done = false
				while not done do
					local chunk = stream:read(1024)
					if chunk == nil then done = true end
				end
				stream:close()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
		{
			Name: "stream with repeat until",
			Code: `
				local http = require("http")
				local req, req_err = http.request()
				if req_err then return end
				local stream, stream_err = req:stream()
				if stream_err then return end
				repeat
					local chunk = stream:read(1024)
				until chunk == nil
				stream:close()
			`,
			WantError: false,
			Stdlib:    true,
			Manifests: manifests,
		},
	}
	testutil.RunCases(t, tests)
}
