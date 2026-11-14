# Theme: reusing the blog's look across repos

Goal: `tools.ciceley.cc` looks like `ciceley.cc` — same rose palette, same fonts, same card
geometry, same dark mode — without editing the blog and without copying markup by hand at runtime.

## What is shared and what is not

| Asset | Shared? | How |
| --- | --- | --- |
| Colours, fonts, radii, spacing | **yes** | compiled CSS (`theme/tools.css`) from the pinned theme + the blog's `custom.scss` |
| Component classes (`.article-list`, `.card`, `.section-title`, `.widget`) | **yes** | they come with the compiled CSS |
| Page skeleton (`baseof.html`) | **no** — ported once | `web/templates/layout.html` |
| Sidebar/footer partials | **no** — reimplemented once | `web/templates/partials/` |
| Hugo | only at build time, for CSS | not a runtime dependency |

## The build step (`make css`)

Inputs, both pinned in this repo:

- `theme/hugo-theme-stack` — **git submodule at tag `v4.0.3`** (the same commit the blog uses;
  `3e123a3`). Read-only.
- `theme/site-scss/custom.scss` — **byte-for-byte copy** of
  `ciceleywebsite/assets/scss/custom.scss` (the pink "Rose & Quill" overrides).

Why a copy and not a reference: the blog is a separate repo with its own release cadence, and a
compile-time dependency on a *path in another working tree* cannot be expressed without a submodule,
a package, or a network fetch. A copy plus a drift check is the cheapest honest option
(`docs/adr/0005-theme-css-pinning.md`).

Compile (deterministic, no load-path guessing):

```bash
tmp=$(mktemp -d)
# 1. theme entry, minus its own `@import "custom.scss"` (which resolves to an empty placeholder)
sed '/@import "custom.scss";/d' theme/hugo-theme-stack/assets/scss/style.scss > "$tmp/entry.scss"
# 2. append OUR palette so it still lands last (it uses the same `:root` cascade trick)
cat theme/site-scss/custom.scss >> "$tmp/entry.scss"
# 3. compile: theme partials resolve via load-path, custom.scss is already inlined
sass --no-source-map --style=compressed \
     --load-path=theme/hugo-theme-stack/assets/scss \
     "$tmp/entry.scss" theme/tools.css
```

Requirements: `sass` (dart-sass) available in `nix develop`. `hugo` is present too, but the blog's
Hugo pipeline (`toCSS`) is not reproducible outside Hugo, hence dart-sass directly.

**Assertion after compiling** (in `make css` and in CI): `theme/tools.css` must contain
`--rose-500` **and** `--accent-color`. If the theme ever renames `custom.scss`, or `sed` stops
matching, the build fails instead of silently shipping an unpainted site.

**Drift check** (CI): run `make css` and fail if `git diff --exit-code theme/tools.css` is dirty.

## The skeleton port

`themes/hugo-theme-stack/layouts/baseof.html` is ~25 lines. Port it to one Go template and keep the
class names identical:

```html
<body class="{{ block "body-class" }}">
  <div class="container main-container flex on-phone--column {{ if .HasWidgets }}extended{{ else }}compact{{ end }}">
    <aside id="sidebar"> … avatar, site name, italic description, nav … </aside>
    <main class="main full-width"> … </main>
  </div>
</body>
```

- Drop: widgets, TOC, comments, search, article lists, pagination, photoswipe, math, mermaid.
- Keep: `.site-avatar .site-logo` (the evil-eye image gets a border ring from `custom.scss`),
  `.site-name`, `.site-description` (italic), `#main-menu` with `.menu-social` hover, footer with
  `since = 2023`.
- Dark mode: `data-scheme` on `<html>`, `localStorage["scheme"]`, fall back to
  `prefers-color-scheme`, and a `<button>` that toggles it. Copy the logic from the theme's
  `_partials/head/colorScheme.html` — but as an **external** JS file, because the CSP forbids inline
  script (`docs/design/security.md` §5).
- Fonts: self-host Playfair Display + Poppins (woff2, `font-display: swap`) under
  `web/static/fonts/`, declared at the top of `theme/site-scss/custom.scss`. The blog loads them from
  Google Fonts; copying that would widen our CSP and leak reader IPs to Google. The blog's own partial
  is `layouts/_partials/head/custom-font.html` if the file list is needed.

## Rules

1. **Never edit `theme/hugo-theme-stack/`.** No patches, no "just one variable".
2. **Never fork the theme's SCSS into `web/static/css/`.** All styling is `tools.css` + the
   `/* tools: */` block in `custom.scss`.
3. **New colours are forbidden.** Use `--accent-color`, `--card-*`, `--body-*`, `--rose-*`.
4. **Re-porting is a review task.** When the blog bumps the theme: bump the submodule to the same
   tag, re-copy `custom.scss`, `git diff` the theme's `baseof.html` against the ported
   `layout.html`, then `make css`. One PR, and say in the description which lines of the skeleton
   changed.

## Visual checklist for M1 (run through it before claiming "themed")

- [ ] index lists tools as cards with the same radius/shadow as blog article cards
- [ ] sidebar shows avatar with the ring, name in Playfair, italic description
- [ ] dark-mode toggle flips `data-scheme` and survives reload
- [ ] focus rings visible on the form controls; form controls use the theme's input styling if any
- [ ] 404 page styled (not bare nginx/Hugo default)
- [ ] no horizontal scroll at 360 px wide (`on-phone--column` behaviour)
- [ ] printed/prerendered HTML is readable without JS (form posts normally)
