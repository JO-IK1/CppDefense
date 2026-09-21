// Package webassets owns the browser UI separately from the HTTP API.
package webassets

import "embed"

// Files contains the production HTML templates and static browser assets.
//
//go:embed templates/*.html static/*
var Files embed.FS
