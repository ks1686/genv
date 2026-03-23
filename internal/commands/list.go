package commands

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ks1686/genv/internal/schema"
)

// List writes a tabular summary of f's packages to w.
// Passing a nil f (file not found) or an empty package list prints a friendly message.
func List(f *schema.GenvFile, w io.Writer) {
	if f == nil || len(f.Packages) == 0 {
		fmt.Fprintln(w, "no packages tracked")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "ID\tVERSION\tPREFER\tMANAGERS")
	fmt.Fprintln(tw, "--\t-------\t------\t--------")

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

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.ID, ver, prefer, managers)
	}

	tw.Flush()
}
