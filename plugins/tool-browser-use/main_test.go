package main

import (
	"reflect"
	"testing"
)

func TestParseEnginesDefaults(t *testing.T) {
	got := parseEngines("")
	want := []string{"google", "duckduckgo", "baidu", "so360"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEngines(\"\") = %v, want %v", got, want)
	}
}

func TestParseEnginesSpecificAndDedupOrder(t *testing.T) {
	got := parseEngines("baidu, google, google, so360")
	want := []string{"baidu", "google", "so360"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEngines = %v, want %v", got, want)
	}
}

func TestParseEnginesFiltersUnknown(t *testing.T) {
	got := parseEngines("google, bing, foobar")
	want := []string{"google"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEngines = %v, want %v", got, want)
	}
}

func TestParseEnginesAllInvalidFallsBackToDefault(t *testing.T) {
	got := parseEngines("bing, yahoo")
	if len(got) != 4 {
		t.Fatalf("all-invalid should fall back to default 4 engines, got %v", got)
	}
}

func TestUnmarshalSearchItemsValid(t *testing.T) {
	raw := `[{"title":"A","url":"http://a","description":"d1"},{"title":"B","url":"http://b"}]`
	items, err := unmarshalSearchItems(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 2 || items[0].Title != "A" || items[0].URL != "http://a" || items[0].Description != "d1" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestUnmarshalSearchItemsInvalid(t *testing.T) {
	if _, err := unmarshalSearchItems(`{broken`); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMergeEngineResultsTagsSourceAndDedup(t *testing.T) {
	items := []SearchResultItem{
		{Title: "X", URL: "http://dup"},
		{Title: "Y", URL: "http://new"},
	}
	seen := map[string]bool{"http://dup": true}
	added, hasNew := mergeEngineResults(items, "baidu", seen)
	if !hasNew || len(added) != 1 || added[0].URL != "http://new" || added[0].Source != "baidu" {
		t.Fatalf("merge: added=%+v hasNew=%v", added, hasNew)
	}
}

func TestMergeEngineResultsAllDup(t *testing.T) {
	items := []SearchResultItem{{Title: "X", URL: "http://dup"}}
	seen := map[string]bool{"http://dup": true}
	added, hasNew := mergeEngineResults(items, "so360", seen)
	if hasNew || len(added) != 0 {
		t.Fatalf("all dup should add nothing, got added=%+v hasNew=%v", added, hasNew)
	}
}
