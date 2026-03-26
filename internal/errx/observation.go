package errx

import (
	"errors"
)

// Observation carries internal error metadata through wrapped errors.
type Observation struct {
	Stage Stage
}

type observationCarrier interface {
	errorObservation() Observation
}

type observedError struct {
	err         error
	observation Observation
}

func (e *observedError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *observedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *observedError) errorObservation() Observation {
	if e == nil {
		return Observation{}
	}
	return e.observation
}

func WithStage(err error, stage Stage) error {
	return WithObservation(err, Observation{Stage: normalizeStage(stage)})
}

func WithObservation(err error, obs Observation) error {
	if err == nil {
		return nil
	}

	base := From(err)
	merged := mergeObservation(base, normalizeObservation(obs))
	if merged == base {
		return err
	}

	return &observedError{
		err:         err,
		observation: merged,
	}
}

func From(err error) Observation {
	if err == nil {
		return Observation{}
	}

	var carrier observationCarrier
	if errors.As(err, &carrier) {
		return carrier.errorObservation()
	}

	return Observation{}
}

func Derive(err error, fallbackStage Stage) Observation {
	obs := From(err)
	if obs.Stage != "" {
		return obs
	}

	stage := normalizeStage(fallbackStage)
	if stage == "" {
		stage = StageProcessing
	}
	return Observation{Stage: stage}
}

func normalizeObservation(obs Observation) Observation {
	obs.Stage = normalizeStage(obs.Stage)
	return obs
}

func mergeObservation(base, override Observation) Observation {
	merged := base

	if override.Stage != "" {
		merged.Stage = override.Stage
	}

	return merged
}
