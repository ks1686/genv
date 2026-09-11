package external

import "fmt"

// ExecutionMode distinguishes an interactive command from --yes and scheduler runs.
type ExecutionMode uint8

const (
	ExecutionInteractive ExecutionMode = iota
	ExecutionAssumeYes
	ExecutionUnattended
)

// PolicyInput describes the risk properties of one external action.
type PolicyInput struct {
	Verified          bool
	Script            bool
	InsecureTransport bool
	BackgroundAllowed bool
	Mode              ExecutionMode
}

// CheckExecutionPolicy rejects external actions that cannot run in the requested mode.
func CheckExecutionPolicy(input PolicyInput) error {
	if input.Mode != ExecutionInteractive && !input.Verified {
		return fmt.Errorf("unverified external packages require interactive acknowledgement")
	}
	if input.Mode != ExecutionInteractive && input.InsecureTransport {
		return fmt.Errorf("external packages using insecure HTTP cannot run unattended")
	}
	if input.Mode != ExecutionInteractive && input.Script && !input.BackgroundAllowed {
		return fmt.Errorf("external installer scripts require allowBackgroundExecution for unattended execution")
	}
	return nil
}
