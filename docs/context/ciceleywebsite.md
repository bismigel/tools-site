# `ciceleywebsite` — the neighbour repo (read-only dependency)

`https://github.com/Ciceley/ciceleywebsite` · local `~/projects/ciceleywebsite`.
This repo **never modifies it** except for the optional one-line menu entry (see below).

## What matters here: the theme and its palette

- Theme: `CaiJimmy/hugo-theme-stack` **v4.0.3**, vendored as a git submodule at
  `themes/hugo-theme-stack` (commit `3e123a3`, tag `v4.0.3`).
- Palette: `assets/scss/custom.scss` in that repo — 100+ lines of CSS custom properties
  (`--rose-50…900`, `--accent-color`, `--body-background`, `--card-*`, `--card-border-radius: 16px`),
  Playfair Display headings + Poppins body, plus a `[data-scheme="dark"]` block.
- The theme's own `assets/scss/custom.scss` is an **empty placeholder**
  (`/* Place your custom SCSS in HUGO_SITE_FOLDER/assets/scss/custom.scss */`), which is why the
  site's file wins under Hugo's asset lookup order.
- Hugo compiles it with `_partials/head/style.html`:
  `resources.Get "scss/style.scss" | toCSS | minify | resources.Fingerprint "sha256"` →
  `public/scss/style.min.<sha>.css` (≈58 kB, all colours driven by custom properties).
- Layout skeleton to port: `themes/hugo-theme-stack/layouts/baseof.html` — `<html>` + `head` +
  `body`, then `.container.main-container.flex.on-phone--column(.extended|.compact)` containing a
  left sidebar block, `main.main.full-width`, and a footer include. ~25 lines.
- Dark-mode toggle logic: `themes/hugo-theme-stack/layouts/_partials/head/colorScheme.html`
  (`data-scheme` attribute, `localStorage`, `prefers-color-scheme`).
- Fonts: `layouts/_partials/head/custom-font.html` in the blog preconnects to Google Fonts and loads
  Playfair Display + Poppins. Reproduce that in the tools layout (or self-host later; note it in
  `docs/design/security.md` because it is third-party).

## Why the CSS is compiled here, not fetched at runtime

`docs/adr/0005-theme-css-pinning.md`. Short version: a cross-site stylesheet URL is a silent
runtime dependency on another deploy (fingerprinted filenames change), so this repo pins the theme
as a submodule and compiles the same input to `theme/tools.css`, committed, with a CI drift check.

Sync contract with the blog: **`theme/site-scss/custom.scss` is a byte-for-byte copy** of
`ciceleywebsite/assets/scss/custom.scss`. When the blog is retheme'd, copy the file and run
`make css`. Add the theme version to both places when it bumps.

## What the blog looks like (so the port matches)

- Sidebar: avatar (`/img/evil-eye.svg`), site title, italic `.site-description`, `main` menu
  (Writing, About) and `social` menu (Email, LinkedIn, GitHub) with icons.
- `hugo.toml` holds `[menu.main]`, `[menu.social]`, `[params.sidebar]`, `[params.colorScheme]`.
- The tools site mirrors this one level deep: same avatar/title styling, a "Tools" list instead of
  posts, no article widgets, no comments, no search.

## The one allowed change over there

```toml
  [[menu.main]]
    identifier = "tools"
    name = "Tools"
    url = "https://tools.ciceley.cc"
    weight = 3
    [menu.main.params]
      icon = "tool"
```

Check `themes/hugo-theme-stack/assets/icons` for an available icon name before choosing one; the
theme ships a tabler icon set. This is a separate PR in `ciceleywebsite`, one line plus an icon
choice, and it is **M4's last step**, not a prerequisite.

## Facts about its delivery (so nobody copies the wrong pattern)

- `deploy.yml` builds Hugo on the runner, `docker build`s `nginx:alpine` + `public/`, pushes to
  GHCR, and `sed`s the tag in `deploy/k8s/deployment.yaml`, then **pushes directly to `main`**.
  That works there — `main` carries `GitHub Action` commits. This repo keeps the template's
  PR-based bump because its `main` protection is unknown
  (`docs/adr/0006-manifest-bump-via-pr.md`).
- Its `nginx.conf` sets `X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy` and 1-year
  immutable caching for fingerprinted assets. The equivalent headers are the Go app's job here
  (`docs/design/security.md`).
- `notion-sync.mjs` + `NOTION_API_KEY` live there. **If a tool needs Notion, it needs its own
  credential path here** (Bitwarden → ExternalSecret), not a copy of the blog's `.env`.

## Divergence to watch

`ciceleywebsite` may bump the theme (Renovate config exists in that repo). When it does, this repo
must bump `theme/hugo-theme-stack` to the same tag, re-copy `custom.scss`, re-diff
`baseof.html` against `web/templates/layout.html`, and run `make css`. Put that on the same PR.
