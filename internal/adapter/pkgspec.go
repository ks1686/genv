package adapter

import "strings"

// jsBasePackageName strips a JavaScript package @version suffix while
// preserving npm-style scoped names such as @scope/pkg.
func jsBasePackageName(spec string) string {
	if strings.HasPrefix(spec, "@") {
		if idx := strings.LastIndex(spec, "@"); idx > 0 && strings.Contains(spec[:idx], "/") {
			return spec[:idx]
		}
		return spec
	}
	if idx := strings.Index(spec, "@"); idx > 0 {
		return spec[:idx]
	}
	return spec
}

func atVersionBaseName(spec string) string {
	if before, _, ok := strings.Cut(spec, "@"); ok {
		return before
	}
	return spec
}

// PythonBasePackageName strips conservative Python requirement markers used by
// global tool specs: extras ([...]) and ==, >=, <=, ~=, !=, or direct-reference @.
func PythonBasePackageName(spec string) string {
	base := strings.TrimSpace(spec)
	if base == "" {
		return base
	}

	if before, _, ok := strings.Cut(base, "["); ok {
		base = before
	}

	for _, marker := range []string{"==", ">=", "<=", "~=", "!="} {
		if before, _, ok := strings.Cut(base, marker); ok {
			return strings.TrimSpace(before)
		}
	}

	if before, _, ok := strings.Cut(base, " @ "); ok {
		return strings.TrimSpace(before)
	}

	return base
}

func nonEmptyLines(output string) []string {
	var result []string
	for line := range strings.SplitSeq(output, "\n") {
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func trimmedNonEmptyLines(output string) []string {
	var result []string
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}
