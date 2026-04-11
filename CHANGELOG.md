# Changelog

All notable changes to this project will be documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning is intended to follow SemVer once `v1.0.0` is released.

## Compatibility Policy

- The public compatibility boundary is the API and HTTP behavior documented in [README.md](./README.md).
- `internal/*` packages, benchmark layouts, test helpers, and implementation details are not public API.
- Before `v1.0.0`, breaking changes may still happen in minor releases, but they should be called out explicitly in this changelog.
- After `v1.0.0`, breaking public API or HTTP contract changes should only ship in a new major version.

## [Unreleased]

### Added

- Added Echo-style request helpers to `reqx` while keeping the library `net/http`-first: raw `From(r).PathParam(...)` / `QueryParam(...)` readers plus typed `reqx.PathParam[T](...)` and `reqx.QueryParam[T](...)`.
- Exposed the same request-helper entry points from the root `hah` facade: `Request`, `From`, `PathParam[T]`, and `QueryParam[T]`.
- Kept the `bind` package focused on DTO binding only; the new request-parameter helpers live in `reqx` instead of the binding layer.

## [v0.3.2] - 2026-04-10

### Highlights

- Hardened test coverage across `bind`, `reqx`, `resp`, and `errx`, adding cases for boundary conditions and edge scenarios.
- Fixed the binding-stage type detection logic for `time.Time` and improved UTF-8 safety when truncating error logs.
- Optimized pointer-field deserialization to reduce unnecessary memory allocations.
- Simplified `errx` error wrapping and stack formatting, and removed unused validation helper code.

## [v0.3.1] - 2026-04-10

### Highlights

- Simplified the success-response API surface in `hah` / `resp` so common JSON writers are easier to call in plain `net/http` handlers and tests.
- Hardened `bind` contracts around body reads, string-source mapping, and edge-case no-op behavior to make request decoding more predictable.
- Refined response writing and error fallback behavior so JSON output, HEAD-like writers, and `ErrorResponder` paths behave more consistently.

### Fixed

- Aligned error-response writing with `net/http` default HEAD semantics instead of keeping a separate HEAD-only branch.
- Fixed `ErrorResponder` fallback so a custom `AsHTTPError` hook returning `nil` still falls back to the default HTTP error mapping.
- Improved degraded write-path diagnostics and related regression coverage for response helpers.

### Breaking

- Removed the unused `*http.Request` parameter from success-response helpers in `hah` / `resp`: `JSON`, `JSONBlob`, `OK`, `Created`, and `NoContent`.

### Documentation

- Refreshed `README.md` as the primary official doc, added feature and installation sections, and aligned supporting docs with the current behavior.

## [v0.3.0] - 2026-04-10

### Highlights

- Added a dedicated `bind` package for pure request-source binding, with public `Bind`, `BindBody`, `BindQueryParams`, `BindPathValues`, `BindHeaders`, `Binder`, `DefaultBinder`, and `BindUnmarshaler` APIs.
- Split request handling responsibilities more clearly: `bind` now owns source-to-DTO mapping, while `reqx` focuses on `BindAndValidate*`, normalization, request-level rules, and validator-based field validation.
- Standardized the default binding contract around `net/http`: `Bind` now runs `path -> query(GET/DELETE/HEAD) -> body`, keeps header binding explicit, and documents a stable JSON-body contract for empty-body no-op, `application/json`, and size/media-type failures.

### Fixed

- Clarified `resp.WriteError(...)` logging behavior so ordinary `4xx` responses still do not emit standalone error logs; only `5xx` failures and error-response write failures produce independent `slog.Default()` records.
- Fixed error-response write-failure diagnostics to log the actual `writeErr` chain instead of falling back to the original `errx.HTTPError` cause, so `"resp: failed to write error response"` now points at the real write-path failure.
- Re-exported `resp.ErrorResponder` and `resp.NewErrorResponder()` from the root `hah` facade so the new customizable error-responder API is available as a public root-package entry point.

### Breaking

- Introduced the standalone `bind` package and moved the default binding implementation out of `reqx`; code that imported `reqx` for binding-only behavior should migrate to `bind`.
- Replaced the previous generic, option-based root `hah.Bind*` wrappers with non-generic thin wrappers over `bind`, and removed the old root aliases for request binding options such as `BindOption`, `BindBodyOption`, `BindQueryParamsOption`, `BindHeadersOption`, and `DefaultMaxBodyBytes`.
- Narrowed `reqx` to the validation composition layer, so its public surface now centers on `BindAndValidate*`, `RequestValidator`, `Normalizer`, `RequireBody`, `InvalidRequest`, and public violations rather than the old bind-only helpers.

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
