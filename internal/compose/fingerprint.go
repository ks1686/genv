package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ks1686/genv/internal/schema"
)

// Fingerprint returns a short, non-secret structural hash of an effective
// environment bundle plus its module selection.
//
// Env values are deliberately excluded (only the variable name and its
// sensitive flag participate) so the fingerprint is safe to record in the
// machine-local lock, print in `genv config`, and log. Everything else —
// resource identity, selection, manager choice, version constraints, service
// argv, hook order — participates, so a structural edit changes the value and a
// credential rotation does not.
// writef appends formatted bytes to a hash. A hash write cannot fail, so the
// error is intentionally discarded — matching the convention used elsewhere in
// this repo (see resolver's fprintf helper).
func writef(h io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(h, format, a...)
}

func Fingerprint(effective *schema.GenvFile, selected []string) string {
	if effective == nil {
		return ""
	}
	h := sha256.New()
	writef(h, "modules\x00%s\x00", strings.Join(selected, ","))

	for _, pkg := range effective.Packages {
		managers := make([]string, 0, len(pkg.Managers))
		for k, v := range pkg.Managers {
			managers = append(managers, k+"="+v)
		}
		sort.Strings(managers)
		writef(h, "package\x00%s\x00%s\x00%s\x00%s\x00%s\x00",
			pkg.ID, pkg.Version, pkg.Prefer, strings.Join(managers, ","), externalFingerprint(pkg))
	}
	for _, name := range sortedKeys(effective.Env) {
		v := effective.Env[name]
		// name + sensitive only: never the value.
		writef(h, "env\x00%s\x00%t\x00", name, v.Sensitive)
	}
	if effective.Shell != nil {
		for _, name := range sortedKeys(effective.Shell.Aliases) {
			v := effective.Shell.Aliases[name]
			writef(h, "alias\x00%s\x00%s\x00%s\x00", name, v.Value, v.Shell)
		}
		for _, name := range sortedKeys(effective.Shell.Functions) {
			v := effective.Shell.Functions[name]
			writef(h, "function\x00%s\x00%s\x00%s\x00", name, v.Body, v.Shell)
		}
		for _, src := range effective.Shell.Source {
			writef(h, "shell-source\x00%s\x00", src)
		}
	}
	for _, name := range sortedKeys(effective.Services) {
		svc := effective.Services[name]
		writef(h, "service\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00",
			name, svc.BrewFormula, strings.Join(svc.Start, "\x01"), strings.Join(svc.Stop, "\x01"),
			strings.Join(svc.Restart, "\x01"), strings.Join(svc.Status, "\x01"))
		if svc.Launchd != nil {
			writef(h, "launchd\x00%s\x00", svc.Launchd.Plist)
		}
		if svc.Systemd != nil {
			writef(h, "systemd\x00%s\x00", svc.Systemd.Unit)
		}
		// The v10 service fields decide whether and how a change is acted on, so
		// editing one has to move the fingerprint: a lock that recorded the old
		// ordering or probe is stale in exactly the way the fingerprint exists to
		// detect. Health check argv participates too, but no probe *output* ever
		// does — this value is written to a machine-local lock and printed.
		writef(h, "requires\x00%s\x00", strings.Join(sortedCopy(svc.Requires), "\x01"))
		writef(h, "watch\x00%s\x00", strings.Join(sortedCopy(svc.Watch), "\x01"))
		writef(h, "restart-policy\x00%s\x00", svc.RestartPolicy)
		if svc.HealthCheck != nil {
			hc := svc.HealthCheck
			writef(h, "health\x00%s\x00%s\x00%s\x00%t\x00",
				strings.Join(hc.Command, "\x01"), hc.Timeout, hc.Interval, hc.AllowBackground)
		}
	}
	if effective.Files != nil {
		for _, link := range sortedByField(effective.Files.Links, func(l schema.FileLink) string { return l.Target }) {
			writef(h, "link\x00%s\x00%s\x00%s\x00%s\x00", link.Source, link.Target, link.Mode, link.Perm)
		}
		for _, tmpl := range sortedByField(effective.Files.Templates, func(t schema.FileTemplate) string { return t.Target }) {
			writef(h, "template\x00%s\x00%s\x00%s\x00", tmpl.Source, tmpl.Target, tmpl.Perm)
		}
		for _, dir := range sortedByField(effective.Files.Dirs, func(d schema.FileDir) string { return d.Target }) {
			writef(h, "dir\x00%s\x00%s\x00", dir.Target, dir.Perm)
		}
	}
	if effective.Hooks != nil {
		for _, phase := range hookPhaseNames {
			for i, hook := range phaseSlice(effective.Hooks, phase) {
				// Hook command/file text participates: changing a hook is a
				// structural change worth a fingerprint difference.
				writef(h, "hook\x00%s\x00%d\x00%s\x00%s\x00%t\x00",
					phase, i, hook.Name, hook.Command+hook.File, hook.ContinueOnError)
			}
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))[:32]
}

func externalFingerprint(pkg schema.Package) string {
	if pkg.External == nil {
		return ""
	}
	data, err := canonicalJSON(pkg.External)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])[:16]
}

func sortedByField[T any](items []T, key func(T) string) []T {
	out := append([]T(nil), items...)
	sort.SliceStable(out, func(i, j int) bool { return key(out[i]) < key(out[j]) })
	return out
}

// sortedCopy sorts a copy so declaration order does not change the hash: two
// specs that declare the same requires list in a different order describe the
// same environment.
func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
