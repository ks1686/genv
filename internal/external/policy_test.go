package external

import "testing"

func TestExecutionPolicy(t *testing.T) {
	tests := []struct {
		name       string
		input      PolicyInput
		wantDenied bool
	}{
		{name: "verified asset interactive", input: PolicyInput{Verified: true}},
		{name: "verified asset unattended", input: PolicyInput{Verified: true, Mode: ExecutionUnattended}},
		{name: "unverified interactive", input: PolicyInput{}},
		{name: "unverified yes denied", input: PolicyInput{Mode: ExecutionAssumeYes}, wantDenied: true},
		{name: "unverified scheduler denied", input: PolicyInput{Mode: ExecutionUnattended}, wantDenied: true},
		{name: "insecure scheduler denied", input: PolicyInput{Verified: true, InsecureTransport: true, Mode: ExecutionUnattended}, wantDenied: true},
		{name: "script yes needs opt in", input: PolicyInput{Verified: true, Script: true, Mode: ExecutionAssumeYes}, wantDenied: true},
		{name: "script scheduler opted in", input: PolicyInput{Verified: true, Script: true, BackgroundAllowed: true, Mode: ExecutionUnattended}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckExecutionPolicy(tt.input)
			if (err != nil) != tt.wantDenied {
				t.Fatalf("CheckExecutionPolicy() error=%v, wantDenied=%v", err, tt.wantDenied)
			}
		})
	}
}
