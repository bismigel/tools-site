// Package toolsite embeds the templates, static assets and generated CSS so the
// binary is self-contained (spec NFR-6.3).
package toolsite

import "embed"

// Web holds web/templates and web/static. The FS root is the repo root, so both
// dev (os.DirFS(".")) and prod (this) resolve the same paths.
//
//go:embed web/templates web/static
var Web embed.FS

// ThemeCSS is the generated stylesheet (make css).
//
//go:embed theme/tools.css
var ThemeCSS []byte
