package errx

import (
	"errors"
	"testing"
)

func TestObservedErrorNilReceiverAndWrappedError(t *testing.T) {
	var nilObserved *observedError
	if got := nilObserved.Error(); got != "" {
		t.Fatalf("nilObserved.Error() = %q, want empty", got)
	}
	if got := nilObserved.Unwrap(); got != nil {
		t.Fatalf("nilObserved.Unwrap() = %#v, want nil", got)
	}
	if got := nilObserved.errorObservation(); got != (Observation{}) {
		t.Fatalf("nilObserved.errorObservation() = %#v, want zero value", got)
	}

	cause := errors.New("boom")
	observed := &observedError{
		err:         cause,
		observation: Observation{Stage: StageProcessing},
	}
	if got := observed.Error(); got != "boom" {
		t.Fatalf("observed.Error() = %q, want boom", got)
	}
	if got := observed.Unwrap(); got != cause {
		t.Fatalf("observed.Unwrap() = %#v, want %#v", got, cause)
	}
	if got := observed.errorObservation(); got != (Observation{Stage: StageProcessing}) {
		t.Fatalf("observed.errorObservation() = %#v, want processing stage", got)
	}
}

func TestWithObservationGuardsAndAvoidsDoubleWrap(t *testing.T) {
	if got := WithObservation(nil, Observation{Stage: StageProcessing}); got != nil {
		t.Fatalf("WithObservation(nil, obs) = %#v, want nil", got)
	}
	if got := From(nil); got != (Observation{}) {
		t.Fatalf("From(nil) = %#v, want zero value", got)
	}

	cause := errors.New("boom")
	observed := WithObservation(cause, Observation{Stage: StageProcessing})
	if observed == cause {
		t.Fatal("WithObservation(cause, obs) returned original error, want wrapped error")
	}

	same := WithObservation(observed, Observation{Stage: StageProcessing})
	if same != observed {
		t.Fatalf("WithObservation(observed, same stage) = %#v, want original observed error %#v", same, observed)
	}
}

func TestDeriveUsesObservedStageOrFallback(t *testing.T) {
	observed := WithStage(errors.New("boom"), StageDecode)

	if got := Derive(observed, StageProcessing); got != (Observation{Stage: StageDecode}) {
		t.Fatalf("Derive(observed, processing) = %#v, want decode stage", got)
	}
	if got := Derive(errors.New("boom"), StageProcessing); got != (Observation{Stage: StageProcessing}) {
		t.Fatalf("Derive(err, processing) = %#v, want processing stage", got)
	}
	if got := Derive(errors.New("boom"), ""); got != (Observation{Stage: StageProcessing}) {
		t.Fatalf("Derive(err, empty) = %#v, want processing stage", got)
	}
}
