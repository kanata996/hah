# Changelog

All notable changes to this project will be documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning is intended to follow SemVer once `v1.0.0` is released.

## Compatibility Policy

- The public compatibility boundary is the API and HTTP behavior documented in [README.md](./README.md).
- `internal/*` packages, benchmark layouts, test helpers, and implementation details are not public API.
- Before `v1.0.0`, breaking changes may still happen in minor releases, but they should be called out explicitly in this changelog.
- After `v1.0.0`, breaking public API or HTTP contract changes should only ship in a new major version.

## [Unreleased]

### Fixed

- Clarified `resp.WriteError(...)` logging behavior so ordinary `4xx` responses still do not emit standalone error logs; only `5xx` failures and error-response write failures produce independent `slog.Default()` records.
- Fixed error-response write-failure diagnostics to log the actual `writeErr` chain instead of falling back to the original `errx.HTTPError` cause, so `"resp: failed to write error response"` now points at the real write-path failure.
- Re-exported `resp.ErrorResponder` and `resp.NewErrorResponder()` from the root `hah` facade so the new customizable error-responder API is available as a public root-package entry point.

### Documentation

- Updated the error-logging documentation to match the current standalone-log contract and the write-failure diagnostic starting point.
- Aligned package `doc.go` API inventories with the current root `hah` facade and `resp.ErrorResponder` public surface.

## [v0.2.0] - 2026-04-07

### Highlights

- Repositioned `hah` as a `net/http`-first JSON API boundary layer with a small root facade over `reqx`, `errx`, and `resp`.
- Added explicit request-side binding and validation APIs for path, query, header, and body inputs with `validator/v10` integration.
- Added explicit response-side success writers and `application/problem+json` error writing with standalone `slog.Default()` 5xx diagnostics.
- Removed all `chi`, router-scoped middleware, request-id, tracing, and access-log dependencies from the public model.

### Breaking

- Removed the `chi`-style render/runtime model built around `WithResponses(...)`, `Render(...)`, `RenderWithMeta(...)`, `RenderEmpty(...)`, `RenderError(...)`, and `Status(...)`.
- Replaced the previous root package shape with a facade over `Bind*`, `BindAndValidate*`, `Param*`, `WriteError`, `JSON*`, `OK`, `Created`, and `NoContent`.
- Removed router-coupled request-id, tracing, access-log, and middleware integration from the library contract.
- Replaced the old shared error model with the dedicated `errx.HTTPError` package boundary and updated error responses to the current `application/problem+json` contract.

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
