# Changelog

All notable changes to this project will be documented in this file.

This project follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning is intended to follow SemVer once `v1.0.0` is released.

## Compatibility Policy

- The public compatibility boundary is the API and HTTP behavior documented in [README.md](./README.md).
- `internal/*` packages, benchmark layouts, test helpers, and implementation details are not public API.
- Before `v1.0.0`, breaking changes may still happen in minor releases, but they should be called out explicitly in this changelog.
- After `v1.0.0`, breaking public API or HTTP contract changes should only ship in a new major version.

## [Unreleased]

## [v0.3.0] - 2026-03-26

### Highlights

- Introduced an explicit business-boundary error model built around `Contract(...)`, immediate `WriteError(w, r, err)`, and explicit `Respond*` success writes.
- Added route-scoped boundary configuration with `ContractOption`, `WithContractErrorMappers(...)`, and `WithContractErrorReporter(...)`.
- Added the `errcode` subpackage as the unified public entry point for shared error-code constants, including the cross-domain codes `resource_not_found`, `already_exists`, `operation_not_allowed`, and `state_conflict`.

### Breaking Changes

- Removed the old runtime-style APIs `HandleErrors(...)`, `Error(...)`, `UseErrorMappers(...)`, `NotFoundHandler()`, and `MethodNotAllowedHandler()`.
- `WriteError(...)` now composes route-scoped `Contract(...)` configuration with call-site `ErrorOption`s. `WithErrorMappers(...)` and `WithErrorReporter(...)` now produce `ErrorOption`s instead of the old runtime `Option`.
- The documented HTTP contract now only covers responses written by `Respond(...)`, `RespondWithMeta(...)`, `RespondEmpty(...)`, and `WriteError(...)`. Router-level `404/405`, outer middleware interceptions, and panic/recover responses are outside the `hah` contract.
- Default error observation now focuses on the business-boundary stages `decode`, `validate`, `processing`, and `write_response`. Router-level `routing` is no longer part of the default `hah` stage model.
- Removed root-package error-code constants from `hah`; shared public codes now live in `errcode`, while `reqx` keeps owning its own request and validation codes.
- Removed the overlapping `errcode` violation constants `too_short`, `too_long`, and `out_of_range`; prefer `min_length`, `max_length`, and `range`.

### Migration Notes

- Replace outer `HandleErrors(...)` installation with `Contract(...)` on the business-boundary route group or handler subtree when you want route-scoped configuration and started-response tracking. `WriteError(...)` itself can still be used without `Contract(...)`.
- Replace `if hah.Error(r, err) { return }` with `if hah.WriteError(w, r, err) { return }`.
- Replace `UseErrorMappers(...)` with `Contract(hah.WithContractErrorMappers(...))` for route-level defaults, or `WriteError(..., hah.WithErrorMappers(...))` for one-shot overrides.
- Import shared error-code constants from `github.com/kanata996/hah/errcode`.

### Docs and Examples

- Refreshed the README to make the business-boundary contract, `Contract(...)` usage, and explicit `WriteError(...)` paths clearer.
- Expanded the technical guide with request-flow, boundary, fail-closed, and error-handling-mode guidance for maintainers.
- Simplified the `chi` and `net/http` examples around the core `Contract(...)` + `WriteError(...)` flow.

### Performance

- Improved `reqx.DecodeQuery(...)` steady-state performance by caching query decode plans and unknown-field lookup state per destination struct type.

## [v0.2.1] - 2026-03-25

### Added

- Added helper constructors for common client-facing HTTP errors: `BadRequest(...)`, `Unauthorized(...)`, `Forbidden(...)`, `NotFound(...)`, `MethodNotAllowed(...)`, `Conflict(...)`, `Gone(...)`, `UnprocessableEntity(...)`, and `TooManyRequests(...)`.

### Changed

- Breaking: simplified the centralized error API from `Error(w, r, err)` to `Error(r, err)`. The response writer is no longer part of the public `Error(...)` signature; use `WriteError(...)` for one-shot immediate handling without middleware.
