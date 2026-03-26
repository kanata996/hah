package hah

import (
	"errors"
	"net/http"

	"github.com/kanata996/hah/errcode"
	"github.com/kanata996/hah/reqx"
)

func mapBoundaryError(err error, cfg writeErrorConfig) *HTTPError {
	if err == nil {
		return defaultInternalError()
	}

	var boundaryErr *HTTPError
	if errors.As(err, &boundaryErr) && boundaryErr != nil {
		return boundaryErr
	}

	var problem *reqx.Problem
	if errors.As(err, &problem) && problem != nil {
		// Accept direct reqx usage as a first-class bridge: callers can bypass the
		// hah facade and RenderError should still normalize reqx problems into the
		// public hah error contract.
		return NewHTTPError(problem.Status(), problem.Code(), problem.Message(), problem.Details()...)
	}

	for _, mapper := range cfg.mappers {
		if mapper == nil {
			continue
		}
		if mapped := mapper(err); mapped != nil {
			return mapped
		}
	}

	return defaultInternalError()
}

func defaultInternalError() *HTTPError {
	return NewHTTPError(
		http.StatusInternalServerError,
		errcode.InternalError,
		"internal server error",
	)
}

func filterErrorMappers(mappers ...ErrorMapper) []ErrorMapper {
	filtered := make([]ErrorMapper, 0, len(mappers))
	for _, mapper := range mappers {
		if mapper != nil {
			filtered = append(filtered, mapper)
		}
	}
	return filtered
}
