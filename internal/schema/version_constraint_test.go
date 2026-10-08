package schema

import (
	"strings"
	"testing"
)

func TestValidVersionConstraint(t *testing.T) {
	valid := []string{"", "*", "1", "1.2.3", "v1.2.3+build.4", "1.2.*", "2026-10.*"}
	for _, constraint := range valid {
		t.Run("valid_"+strings.ReplaceAll(constraint, "*", "star"), func(t *testing.T) {
			if !ValidVersionConstraint(constraint) {
				t.Errorf("ValidVersionConstraint(%q) = false, want true", constraint)
			}
		})
	}

	invalid := []string{
		"-1.2.3",
		"1 2",
		"1\t2",
		"1\n2",
		"1*",
		"1.*.2",
		"*.1",
		"1.*.*",
		strings.Repeat("1", MaxVersionConstraintBytes+1),
	}
	for _, constraint := range invalid {
		t.Run("invalid", func(t *testing.T) {
			if ValidVersionConstraint(constraint) {
				t.Errorf("ValidVersionConstraint(%q) = true, want false", constraint)
			}
		})
	}
}

func TestParseAndValidateRejectsInvalidVersionConstraints(t *testing.T) {
	for _, constraint := range []string{"-1.2.3", "1 2", "1*", strings.Repeat("1", MaxVersionConstraintBytes+1)} {
		t.Run("invalid", func(t *testing.T) {
			json := `{"schemaVersion":"1","packages":[{"id":"git","version":"` + constraint + `"}]}`
			_, errs, err := ParseAndValidate([]byte(json))
			if err != nil {
				t.Fatalf("ParseAndValidate fatal error: %v", err)
			}
			found := false
			for _, validationErr := range errs {
				if validationErr.Field == "packages[0].version" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("validation errors = %v, want packages[0].version", errs)
			}
		})
	}
}
