package io

import "github.com/wippyai/go-lua/types/typ"

// LookupManifest resolves a manifest by path.
//
// Resolution policy is canonical across checker layers:
//  1. Direct lookup via Manifest(path)
//  2. Fallback lookup via Imports()[path]
func LookupManifest(manifests ManifestQuerier, path string) *Manifest {
	if manifests == nil || path == "" {
		return nil
	}
	manifest := manifests.Manifest(path)
	if manifest == nil {
		if imports := manifests.Imports(); imports != nil {
			manifest = imports[path]
		}
	}
	return manifest
}

// LookupEnrichedExport resolves the manifest by path and returns its enriched export type.
func LookupEnrichedExport(manifests ManifestQuerier, path string) typ.Type {
	manifest := LookupManifest(manifests, path)
	if manifest == nil {
		return nil
	}
	return manifest.EnrichedExport()
}
