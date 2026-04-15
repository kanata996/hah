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

- Merged the standalone `bind` package into `reqx`. `reqx` now owns `BindQuery(...)`, `BindBody(...)`, `BindUnmarshaler`, and the public body decode error-code constants; direct `bind` imports should migrate to `reqx`.
- Removed the redundant `Query(...).Strings()` alias from `reqx` and `hah`; repeated query values now use the single canonical `Values()` entry point.
- Narrowed `bind` to two explicit public entry points: `BindQuery(...)` and `BindBody(...)`. Removed `bind.Bind(...)`, `bind.BindPathValues(...)`, and `bind.BindHeaders(...)`, and renamed `bind.BindQueryParams(...)` to `bind.BindQuery(...)`.
- Narrowed the root `hah` facade accordingly: removed `hah.Bind(...)`, `hah.BindPathValues(...)`, and `hah.BindHeaders(...)`, and renamed `hah.BindQueryParams(...)` to `hah.BindQuery(...)`.
- Removed the built-in `validator/v10` integration from the core `hah` module.
- Removed `hah.BindAndValidate(...)` and `reqx.Validate(...)`; request binding now stops at `Bind*`, and post-bind validation is fully caller-defined.
- Removed the `reqx.Normalizer` and `reqx.RequestValidator` DTO lifecycle hooks from the public API.
- Removed `reqx.ViolationInRequest`; public invalid-request helpers now expose only concrete input locations (`body`, `query`, `path`, `header`).
- Removed `resp.AsHTTPError(...)`, `hah.AsHTTPError(...)`, `resp.ErrorResponder`, `resp.NewErrorResponder()`, and the corresponding root-package facade exports. `resp` now exposes only concrete response writers, and caller-owned logging or error classification stays outside the package.
- Removed the request parameter from `resp.WriteError(...)` and `hah.WriteError(...)`; the response writer alone now carries the write semantics.
- Removed the public `resp.ErrorWriteDegraded` type. If a public error payload cannot be JSON-encoded, `WriteError(...)` now returns that encoding error directly instead of attempting a degraded fallback response.

### Changed

- Repositioned `reqx` around request helpers and explicit invalid-request helpers such as `RequireBody(...)` and `InvalidRequest(...)`.
- Reworked the bundled `net/http` and `chi` examples to demonstrate explicit `Path/Query` helpers plus `BindQuery(...)` / `BindBody(...)` flows instead of mixed-source binding.
- Clarified `reqx.Path(...)` / `reqx.Query(...)` as request-side core APIs and tightened their contract-focused documentation/testing.
- Tightened `reqx.BindQuery(...)` to fail fast on unsupported root targets and invalid DTO/tag shapes instead of silently no-oping or reporting those programmer errors as client `400` responses.
- Repositioned `resp` as a pure response-boundary package: it now exposes only success-response and error-response writers without choosing an application logging policy.
- Simplified `resp` internals around a stricter `net/http`-first model: `WriteError(...)` no longer inspects wrapped `ResponseWriter` state and should be called before a handler starts writing the response.

### Documentation

- Rewrote `README.md`, `REQUESTS.md`, package docs, and testing guidance around binding-first request handling with user-chosen validation.
- Updated response-side docs and examples to use `WriteError(...)` directly and keep caller-owned logging outside `resp`.

## [v0.4.4] - 2026-04-14

### Fixed

- Preserved `HasBody(...)` read failures across repeated body-presence probes so binding and validation paths no longer lose the original I/O error after the first check.
- Hardened `bind` string-source writes for pointer fields so failed decodes do not leave newly allocated partial values behind, and made header binding merge canonical and non-canonical keys deterministically.
- Tightened `reqx.Validate(...)` error handling so unexpected validator failures are returned directly instead of being assumed to be field-violation lists.
- Normalized fallback `errx.HTTPError` titles for unknown `4xx` statuses to `Client Error` instead of incorrectly falling back to the `500` title.
- Improved `resp` error-response handling by checking deeper wrapped `http.ResponseWriter` chains before rewriting started responses and by simplifying the final `application/problem+json` write path.

### Testing

- Expanded contract-focused regression coverage across `bind`, `reqx`, `resp`, `errx`, and internal request helpers, including new fuzz coverage for path pattern helpers.

## [v0.4.3] - 2026-04-13

### Changed

- Bumped the main module and bundled examples to Go `1.25.9`.

## [v0.4.2] - 2026-04-13

### Breaking

- Removed the public `reqx.PathParam[T](...)` / `reqx.QueryParam[T](...)` APIs and their root `hah` facade re-exports. Single-field request access now converges on `Path(...)` / `Query(...)`, while complex decoding continues to use `bind`.
- Split the single-parameter request helper roots into `reqx.PathParam` and `reqx.QueryParam`, and updated the root `hah` facade to re-export those two distinct builder types instead of the old shared `Param` root.
- Narrowed `reqx.Path(...)` / `hah.Path(...)` to resource-identifier-oriented types only: `String`, `UUID`, `Int`, `Int64`, `Uint`, and `Uint64`. Time, duration, float, and bool builders are now query-only.

### Added

- Added `Uint64()` to the `reqx.Path(...)` / `reqx.Query(...)` single-parameter builders.
- Added query-only helpers to `reqx.Query(...)` / `hah.Query(...)`, including `Duration()` and raw repeated-value readers `Values()` / `Strings()`.

### Documentation

- Expanded `REQUESTS.md` and refreshed `README.md` to clarify the boundary between `Path` and `Query`: `Path` stays narrow and resource-oriented, while `Query` carries the wider parameter surface, including raw repeated-key access before you need full `bind.Bind*` decoding.

## [v0.4.1] - 2026-04-13

### Added

- Added `reqx.Path(...)` and `reqx.Query(...)` as single-parameter path/query readers with common validation shortcuts such as `Required`, `Default`, `Min`, `Max`, `OneOf`, `Match`, `Before`, `After`, and `Check`.
- Re-exported `Path(...)` and `Query(...)` from the root `hah` facade.
- Re-exported `BindQueryParams(...)`, `BindPathValues(...)`, and `BindHeaders(...)` from the root `hah` facade so common source-specific binding flows no longer need a separate `bind` import.

### Documentation

- Reframed the request-input docs around two main paths: single-parameter `reqx.Path/Query` builders and DTO-oriented `bind` / `BindAndValidate`.

## [v0.4.0] - 2026-04-11

### Breaking

- Simplified the root `hah` facade to the main happy-path APIs: `Bind`, `BindBody`, `PathParam`, `QueryParam`, `BindAndValidate`, `RequireBody`, and response helpers. Source-specific bind/validate entry points now live in `bind` and `reqx`.
- Removed `bind.Binder` and `bind.DefaultBinder`; use the package-level `bind.Bind(...)` entry point directly.
- Removed `reqx.Request`, `reqx.From`, and the source-specific `reqx.BindAndValidateBody/Query/Path/Headers` wrappers.

### Added

- Added `reqx.Validate(r, target, reqx.Source*)` for source-aware validation after explicit `bind.Bind*` calls.

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
