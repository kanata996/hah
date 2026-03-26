# Changelog

All notable changes to this project will be documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning is intended to follow SemVer once `v1.0.0` is released.

## Compatibility Policy

- The public compatibility boundary is the API and HTTP behavior documented in [README.md](./README.md).
- `internal/*` packages, benchmark layouts, test helpers, and implementation details are not public API.
- Before `v1.0.0`, breaking changes may still happen in minor releases, but they should be called out explicitly in this changelog.
- After `v1.0.0`, breaking public API or HTTP contract changes should only ship in a new major version.

## [Unreleased]

## [v0.1.1] - 2026-03-27

### Highlights

- Simplified `hah` into a render-first JSON API layer built around `WithResponses(...)`, `Render(...)`, `RenderWithMeta(...)`, `RenderEmpty(...)`, `RenderError(...)`, and `Status(...)`.
- Added route-scoped response defaults so a subtree can share `ErrorMappers(...)`, `ErrorReporter(...)`, and `SuccessStatus(...)` without introducing a hidden runtime.
- Updated docs and examples to match the new API and the new `chi`-style render model.

### Breaking

- Replace `WriteError(...)` with `RenderError(...)`.
- Replace `Respond(...)`, `RespondWithMeta(...)`, and `RespondEmpty(...)` with `Render(...)`, `RenderWithMeta(...)`, and `RenderEmpty(...)`.
- Replace `Contract(...)` with `WithResponses(...)`.
- Code that relied on `hah` wrapping `http.ResponseWriter` must migrate to explicit render helpers; the old response-writer tracking path and `internal/resp` implementation have been removed.

## [v0.1.0] - 2026-03-26

### Highlights

- First public release of `hah`.
- Positioned `hah` as a `net/http`-compatible business-boundary JSON API contract library that can be used directly or extended by framework-specific adapters.
- Included the current explicit business-boundary model built around `Contract(...)`, immediate `WriteError(w, r, err)`, explicit `Respond*` success writes, route-scoped mappers/reporters, `reqx`, `errcode`, and request-id bridging.
