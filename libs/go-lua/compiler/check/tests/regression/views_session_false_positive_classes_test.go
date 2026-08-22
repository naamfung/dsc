package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard for views class:
// setmetatable instance fields assigned in constructor must be visible as
// concrete fields across module boundaries, even when metatable has methods.
func TestModuleExport_InstanceFieldOverridesMetatableMethod(t *testing.T) {
	registrySource := `
		local id_mt = {}
		id_mt.__index = id_mt

		function id_mt:ns()
			return "method"
		end

		function id_mt:name()
			return "method"
		end

		local registry = {}

		function registry.parse_id(_raw)
			local self = setmetatable({}, id_mt)
			self.ns = "wippy.views"
			self.name = "page"
			return self
		end

		return registry
	`

	registryModule := testutil.CheckAndExport(registrySource, "registry", testutil.WithStdlib())
	if registryModule.HasError() {
		t.Fatalf("registry module should export cleanly, got: %v", testutil.ErrorMessages(registryModule.Errors))
	}

	consumerSource := `
		local registry = require("registry")
		local full_id = registry.parse_id("wippy.views:page")
		local s: string = full_id.ns .. ":" .. full_id.name
		return s
	`

	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("registry", registryModule),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard for views test class:
// loop bounds i=1..#arr-1 should prove arr[i] and arr[i+1] are non-nil.
func TestForLoopLenMinusOne_NarrowsIndexedElementsNonNil(t *testing.T) {
	source := `
		type Page = { order: number, title: string }

		local pages: {Page} = {
			{ order = 1, title = "a" },
			{ order = 2, title = "b" },
		}

		for i = 1, #pages - 1 do
			local a = pages[i]
			local b = pages[i + 1]
			if a.order == b.order then
				local ok1: boolean = a.title <= b.title
				if not ok1 then
					return false
				end
			else
				local ok2: boolean = a.order < b.order
				if not ok2 then
					return false
				end
			end
		end

		return true
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard for dynamic sequence loops:
// for i=1,#arr-1 over a sequence built by table.insert should allow arr[i]
// and arr[i+1] field access without nil-index errors.
func TestForLoopLenMinusOne_DynamicSequenceIndexing(t *testing.T) {
	source := `
		type Item = { order: number, title: string }

		local items = {}
		table.insert(items, { order = 1, title = "a" })
		table.insert(items, { order = 2, title = "b" })
		table.insert(items, { order = 3, title = "c" })

		for i = 1, #items - 1 do
			local a = items[i]
			local b = items[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end

		return true
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard for views/page_registry_test shape:
// sequence built via table.insert inside a source-loop then indexed via
// i=1..#arr-1 should not collapse element reads to nil.
func TestForLoopLenMinusOne_FilteredSequenceFromLoop(t *testing.T) {
	source := `
		type Item = { id: string, order: number, title: string }

		local source_items: {Item} = {
			{ id = "a", order = 1, title = "A" },
			{ id = "b", order = 2, title = "B" },
			{ id = "c", order = 3, title = "C" },
		}

		local filtered = {}
		for _, item in ipairs(source_items) do
			if item.id ~= "" then
				table.insert(filtered, item)
			end
		end

		for i = 1, #filtered - 1 do
			local a = filtered[i]
			local b = filtered[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end

		return true
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestForLoopLenMinusOne_FilteredSequenceFromOptionalSource(t *testing.T) {
	source := `
		type Item = { id: string, order: number, title: string }

		local function find_all(ok: boolean): ({Item}?, string?)
			if not ok then
				return nil, "err"
			end
			return {
				{ id = "a", order = 1, title = "A" },
				{ id = "b", order = 2, title = "B" },
				{ id = "c", order = 3, title = "C" },
			}, nil
		end

		local pages, err = find_all(true)
		if err then
			return false
		end

		local filtered = {}
		for _, item in ipairs(pages) do
			if item.id:find("^a") or item.id:find("^b") then
				table.insert(filtered, item)
			end
		end

		for i = 1, #filtered - 1 do
			local a = filtered[i]
			local b = filtered[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end

		return true
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestModuleExportedFindAll_PreservesSequenceElementShape(t *testing.T) {
	providerSource := `
		type Item = { id: string, order: number, title: string }

		local pages = {}

		function pages.find_all(): ({Item}?, string?)
			local entries: {Item} = {
				{ id = "a", order = 1, title = "A" },
				{ id = "b", order = 2, title = "B" },
				{ id = "c", order = 3, title = "C" },
			}
			return entries, nil
		end

		return pages
	`

	provider := testutil.CheckAndExport(providerSource, "page_registry", testutil.WithStdlib())
	if provider.HasError() {
		t.Fatalf("provider should export cleanly, got: %v", testutil.ErrorMessages(provider.Errors))
	}

	consumerSource := `
		local page_registry = require("page_registry")

		local pages, err = page_registry.find_all()
		if err then
			return false
		end

		local test_pages = {}
		for _, page in ipairs(pages) do
			if page.id:find("^a") or page.id:find("^b") then
				table.insert(test_pages, page)
			end
		end

		for i = 1, #test_pages - 1 do
			local a = test_pages[i]
			local b = test_pages[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end
		return true
	`

	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("page_registry", provider),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestNestedCallback_ForLoopLenMinusOneFilteredSequence(t *testing.T) {
	source := `
		type Item = { id: string, order: number, title: string }

		local test = {}
		function test.it(_name: string, fn: fun())
			fn()
		end

		local function find_all(): ({Item}?, string?)
			return {
				{ id = "a", order = 1, title = "A" },
				{ id = "b", order = 2, title = "B" },
				{ id = "c", order = 3, title = "C" },
			}, nil
		end

		test.it("sorts by order then title", function()
			local pages, err = find_all()
			if err then
				return
			end

			local filtered = {}
			for _, page in ipairs(pages) do
				if page.id:find("^a") or page.id:find("^b") then
					table.insert(filtered, page)
				end
			end

			for i = 1, #filtered - 1 do
				local a = filtered[i]
				local b = filtered[i + 1]
				if a.order == b.order then
					local _ok1: boolean = a.title <= b.title
				else
					local _ok2: boolean = a.order < b.order
				end
			end
		end)

		return true
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Regression guard for mixed-arity return joins:
// function returning (nil, err) on one path and table-only on success paths must
// preserve non-nil table in first return slot.
func TestReturnJoin_MixedArityPreservesPrimaryValue(t *testing.T) {
	source := `
		type Item = { id: string, order: number, title: string }

		local function find_all(kind: string): ({Item}?, string?)
			if kind == "err" then
				return nil, "err"
			end
			if kind == "empty" then
				return {}
			end
			local pages: {Item} = {
				{ id = "a", order = 1, title = "A" },
				{ id = "b", order = 2, title = "B" },
			}
			return pages
		end

		local pages, err = find_all("ok")
		if err then
			return false
		end

		local filtered = {}
		for _, page in ipairs(pages) do
			if page.id ~= "" then
				table.insert(filtered, page)
			end
		end

		for i = 1, #filtered - 1 do
			local a = filtered[i]
			local b = filtered[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end

		return true
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestModuleReturnInference_UnannotatedMixedReturnsKeepArrayShape(t *testing.T) {
	providerSource := `
		local pages = {}

		function pages.find_all_components(mode)
			if mode == "err" then
				return nil, "boom"
			end

			local entries = {}
			if mode == "empty" then
				return {}
			end

			table.insert(entries, {
				id = "x",
				name = "X",
				title = "X",
				icon = "",
				order = 1,
				group = "",
				group_icon = "",
				group_order = 1,
				group_placement = "default",
				secure = false,
				announced = true,
				kind = "component",
			})

			return entries
		end

		return pages
	`

	provider := testutil.CheckAndExport(providerSource, "page_registry", testutil.WithStdlib())
	if provider.HasError() {
		t.Fatalf("provider should export cleanly, got: %v", testutil.ErrorMessages(provider.Errors))
	}

	consumerSource := `
		type ComponentResponse = {
			id: string,
			name: string,
			title: string,
			icon: string,
			order: number,
			group: string,
			group_icon: string,
			group_order: number,
			group_placement: string,
			secure: boolean,
			announced: boolean,
			kind: string,
		}

		local page_registry = require("page_registry")
		local all_components, err = page_registry.find_all_components("ok")
		if err then
			return false
		end

		local components: {ComponentResponse} = {}
		for _, component in ipairs(all_components) do
			local info: ComponentResponse = {
				id = component.id,
				name = component.name,
				title = component.title,
				icon = component.icon,
				order = component.order,
				group = component.group,
				group_icon = component.group_icon,
				group_order = component.group_order,
				group_placement = component.group_placement,
				secure = component.secure,
				announced = component.announced,
				kind = component.kind,
			}
			table.insert(components, info)
		end
		return true
	`

	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("page_registry", provider),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestModuleReturnInference_EmptyTableSuccessNotCollapsedToNil(t *testing.T) {
	providerSource := `
		local pages = {}
		function pages.find_all(mode)
			if mode == "err" then
				return nil, "boom"
			end
			local out = {}
			return out
		end
		return pages
	`
	provider := testutil.CheckAndExport(providerSource, "page_registry", testutil.WithStdlib())
	if provider.HasError() {
		t.Fatalf("provider should export cleanly, got: %v", testutil.ErrorMessages(provider.Errors))
	}

	consumerSource := `
		local page_registry = require("page_registry")
		local pages, err = page_registry.find_all("ok")
		if err then
			return false
		end
		for _, _v in ipairs(pages) do
		end
		return true
	`
	result := testutil.Check(
		consumerSource,
		testutil.WithStdlib(),
		testutil.WithModule("page_registry", provider),
	)
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFindAllWithAnyMeta_FilterThenAdjacentIndexing(t *testing.T) {
	source := `
		local function extract_page_info(entry)
			local meta = entry.meta
			return {
				id = entry.id,
				name = meta.name or "",
				title = meta.title or "",
				order = meta.order or 9999,
				group = meta.group or "",
				group_order = meta.group_order or 9999,
				group_placement = meta.group_placement or "default",
				secure = meta.secure or false,
				announced = meta.announced or false,
				kind = "component",
			}
		end

		local function find_all(mode)
			if mode == "err" then
				return nil, "err"
			end
			local entries = {
				{ id = "ns:a", meta = ({ name = "a", title = "A", order = 1, announced = true } :: any) },
				{ id = "ns:b", meta = ({ name = "b", title = "B", order = 2, announced = true } :: any) },
				{ id = "ns:c", meta = ({ name = "c", title = "C", order = 3, announced = true } :: any) },
			}
			if mode == "empty" then
				return {}
			end
			local pages_list = {}
			for _, entry in ipairs(entries) do
				if entry.meta then
					table.insert(pages_list, extract_page_info(entry))
				end
			end
			table.sort(pages_list, function(a, b)
				if a.order == b.order then
					return a.title < b.title
				end
				return a.order < b.order
			end)
			return pages_list
		end

		local pages, err = find_all("ok")
		if err then
			return false
		end

		local test_pages = {}
		for _, page in ipairs(pages) do
			if page.id:find("^ns:") then
				table.insert(test_pages, page)
			end
		end

		for i = 1, #test_pages - 1 do
			local a = test_pages[i]
			local b = test_pages[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFindAllWithAnyMeta_NoSort_FilterThenAdjacentIndexing(t *testing.T) {
	source := `
		local function extract_page_info(entry)
			local meta = entry.meta
			return {
				id = entry.id,
				name = meta.name or "",
				title = meta.title or "",
				order = meta.order or 9999,
				group = meta.group or "",
				group_order = meta.group_order or 9999,
				group_placement = meta.group_placement or "default",
				secure = meta.secure or false,
				announced = meta.announced or false,
				kind = "component",
			}
		end

		local function find_all(mode)
			if mode == "err" then
				return nil, "err"
			end
			local entries = {
				{ id = "ns:a", meta = ({ name = "a", title = "A", order = 1, announced = true } :: any) },
				{ id = "ns:b", meta = ({ name = "b", title = "B", order = 2, announced = true } :: any) },
				{ id = "ns:c", meta = ({ name = "c", title = "C", order = 3, announced = true } :: any) },
			}
			if mode == "empty" then
				return {}
			end
			local pages_list = {}
			for _, entry in ipairs(entries) do
				if entry.meta then
					table.insert(pages_list, extract_page_info(entry))
				end
			end
			return pages_list
		end

		local pages, err = find_all("ok")
		if err then
			return false
		end

		local test_pages = {}
		for _, page in ipairs(pages) do
			if page.id:find("^ns:") then
				table.insert(test_pages, page)
			end
		end

		for i = 1, #test_pages - 1 do
			local a = test_pages[i]
			local b = test_pages[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFindAllWithAnyMeta_NoEmptyBranch_FilterThenAdjacentIndexing(t *testing.T) {
	source := `
		local function extract_page_info(entry)
			local meta = entry.meta
			return {
				id = entry.id,
				name = meta.name or "",
				title = meta.title or "",
				order = meta.order or 9999,
				kind = "component",
			}
		end

		local function find_all(mode)
			if mode == "err" then
				return nil, "err"
			end
			local entries = {
				{ id = "ns:a", meta = ({ name = "a", title = "A", order = 1 } :: any) },
				{ id = "ns:b", meta = ({ name = "b", title = "B", order = 2 } :: any) },
				{ id = "ns:c", meta = ({ name = "c", title = "C", order = 3 } :: any) },
			}
			local pages_list = {}
			for _, entry in ipairs(entries) do
				if entry.meta then
					table.insert(pages_list, extract_page_info(entry))
				end
			end
			return pages_list
		end

		local pages, err = find_all("ok")
		if err then
			return false
		end

		local test_pages = {}
		for _, page in ipairs(pages) do
			if page.id:find("^ns:") then
				table.insert(test_pages, page)
			end
		end

		for i = 1, #test_pages - 1 do
			local a = test_pages[i]
			local b = test_pages[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFindAllWithTypedMeta_NoEmptyBranch_FilterThenAdjacentIndexing(t *testing.T) {
	source := `
		local function extract_page_info(entry)
			local meta = entry.meta
			return {
				id = entry.id,
				name = meta.name or "",
				title = meta.title or "",
				order = meta.order or 9999,
				kind = "component",
			}
		end

		local function find_all(mode)
			if mode == "err" then
				return nil, "err"
			end
			local entries = {
				{ id = "ns:a", meta = { name = "a", title = "A", order = 1 } },
				{ id = "ns:b", meta = { name = "b", title = "B", order = 2 } },
				{ id = "ns:c", meta = { name = "c", title = "C", order = 3 } },
			}
			local pages_list = {}
			for _, entry in ipairs(entries) do
				if entry.meta then
					table.insert(pages_list, extract_page_info(entry))
				end
			end
			return pages_list
		end

		local pages, err = find_all("ok")
		if err then
			return false
		end

		local test_pages = {}
		for _, page in ipairs(pages) do
			if page.id:find("^ns:") then
				table.insert(test_pages, page)
			end
		end

		for i = 1, #test_pages - 1 do
			local a = test_pages[i]
			local b = test_pages[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestLoopInsert_FromAnyIterable_WidensSequence(t *testing.T) {
	source := `
		local pages = {} :: any
		local test_pages = {}
		for _, page in ipairs(pages) do
			table.insert(test_pages, page)
		end

		for i = 1, #test_pages - 1 do
			local a = test_pages[i]
			local b = test_pages[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDirectInsert_AnyValue_WidensSequence(t *testing.T) {
	source := `
		local page = {} :: any
		local test_pages = {}
		table.insert(test_pages, page)

		for i = 1, #test_pages - 1 do
			local a = test_pages[i]
			local b = test_pages[i + 1]
			if a.order == b.order then
				local _ok1: boolean = a.title <= b.title
			else
				local _ok2: boolean = a.order < b.order
			end
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIpairsOverAny_ValueNotCollapsedToNil(t *testing.T) {
	source := `
		local pages = {} :: any
		for _, page in ipairs(pages) do
			local _id = page.id
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestIpairsOverAny_ValueNotTypedAsNil(t *testing.T) {
	source := `
		local pages = {} :: any
		for _, page in ipairs(pages) do
			local _n: nil = page
		end
		return true
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected error assigning iterator value to nil, got none")
	}
}

// Regression guard for session class:
// context.reader methods must survive callback registration/invocation flow.
func TestHandlerCallbackFlow_PreservesContextReaderMethods(t *testing.T) {
	source := `
		local prompt_builder = {}
		function prompt_builder.from_session(session)
			local _msgs, _err = session:messages():all()
			local _st = session:state()
			return {}, nil
		end

		local handlers = {}
		function handlers.agent_step(ctx, _op)
			local _builder, _err = prompt_builder.from_session(ctx.reader)
			local session_context, ctx_err = ctx.reader:get_full_context()
			if ctx_err then
				session_context = {}
			end
			local session_data = ctx.reader:state()
			return session_context, session_data
		end

		local reader_mt = {}
		reader_mt.__index = reader_mt
		function reader_mt:get_full_context()
			return {}, nil
		end
		function reader_mt:state()
			return { meta = {} }
		end
		function reader_mt:messages()
			return {
				all = function()
					return {}, nil
				end
			}
		end

		local function open_reader()
			local self = setmetatable({}, reader_mt)
			return self, nil
		end

		local function run()
			local r, _ = open_reader()
			local context = { reader = r }
			local handler = handlers.agent_step
			return handler(context, {})
		end

		run()
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
