package errx

import "strings"

// Stage identifies where an internal error was observed.
type Stage string

const (
	StageDecode        Stage = "decode"
	StageValidate      Stage = "validate"
	StageProcessing    Stage = "processing"
	StageWriteResponse Stage = "write_response"
)

func (s Stage) String() string {
	return string(s)
}

func normalizeStage(stage Stage) Stage {
	return Stage(strings.TrimSpace(stage.String()))
}
