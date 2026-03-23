package commands

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ks1686/genv/internal/schema"
)

// fprintf/fPrintln wrap fmt write functions to discard unactionable I/O errors.
func fprintf(w io.Writer, format string, a ...any)  { _, _ = fmt.Fprintf(w, format, a...) }
func fPrintln(w io.Writer, a ...any)                { _, _ = fmt.Fprintln(w, a...) }

// List writes a tabular summary of f's packages to w.
// Passing a nil f (file not found) or an empty package list prints a friendly message.
func List(f *schema.GenvFile, w io.Writer) {
	if f == nil || len(f.Packages) == 0 {
		fPrintln(w, "no packages tracked")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fPrintln(tw, "ID\tVERSION\tPREFER\tMANAGERS")
	fPrintln(tw, "--\t-------\t------\t--------")

	for _, p := range f.Packages {
		ver := p.Version
		if ver == "" {
			ver = "*"
		}

		prefer := p.Prefer
		if prefer == "" {
			prefer = "-"
		}

		managers := "-"
		if len(p.Managers) > 0 {
			keys := make([]string, 0, len(p.Managers))
			for k := range p.Managers {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, k+"="+p.Managers[k])
			}
			managers = strings.Join(parts, ", ")
		}

		fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, ver, prefer, managers)
	}

	_ = tw.Flush()
}
