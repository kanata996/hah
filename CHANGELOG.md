# Changelog

All notable changes to this project will be documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning is intended to follow SemVer once `v1.0.0` is released.

## Compatibility Policy

- The public compatibility boundary is the API and HTTP behavior documented in [README.md](./README.md).
- `internal/*` packages, benchmark layouts, test helpers, and implementation details are not public API.
- Before `v1.0.0`, breaking changes may still happen in minor releases, but they should be called out explicitly in this changelog.
- After `v1.0.0`, breaking public API or HTTP contract changes should only ship in a new major version.

## [Unreleased]

### Breaking

- Calls using `WriteError(...)` must migrate to `RenderError(...)`.
- Calls using `Respond(...)`, `RespondWithMeta(...)`, or `RespondEmpty(...)` must migrate to `Render(...)`, `RenderWithMeta(...)`, or `RenderEmpty(...)`.
- Code that relied on `hah` wrapping `http.ResponseWriter` should migrate to explicit `hah` render helpers; the old transparent writer-tracking model has been removed.

### Changed

- Reworked the public response API to a render-first model built around `WithResponses(...)`, `Status(...)`, `Render(...)`, `RenderWithMeta(...)`, `RenderEmpty(...)`, and `RenderError(...)`.
- Simplified the business-boundary runtime so `WithResponses(...)` now carries route-scoped mapper / reporter configuration, minimal success-status defaults, and shared request state, without wrapping `http.ResponseWriter`.
- Kept request-scoped error reporting and request-id reuse semantics while redefining `ResponseStarted` to mean "a hah-managed render path has already begun".
- Updated examples, tests, and user-facing docs to use the new render-first flow instead of the previous `WriteError(...)` / `Respond*` model.
- Narrowed the public response surface to the common JSON API path instead of exposing less-common streaming / upgrade render helpers up front.
- Added `SuccessStatus(...)` as the first route-scoped success-side default so `WithResponses(...)` can predeclare common success status codes without hiding explicit `Render*` calls.

### Removed

- Removed the old `WriteError(...)` error-writing API.
- Removed the old `Respond(...)`, `RespondWithMeta(...)`, and `RespondEmpty(...)` success-writing APIs.
- Removed `ResponseWriter` tracking, optional-interface passthrough wrappers, and the old `internal/resp` implementation path.

## [v0.1.0] - 2026-03-26

### Highlights

- First public release of `hah`.
- Positioned `hah` as a `net/http`-compatible business-boundary JSON API contract library that can be used directly or extended by framework-specific adapters.
- Included the current explicit business-boundary model built around `Contract(...)`, immediate `WriteError(w, r, err)`, explicit `Respond*` success writes, route-scoped mappers/reporters, `reqx`, `errcode`, and request-id bridging.
