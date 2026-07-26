package commands

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ks1686/genv/internal/schema"
)

// ErrServiceNotFound is returned when the named service is not in the spec.
var ErrServiceNotFound = errors.New("service not found in spec")

// ServiceAdd adds or updates the service in f's services block.
// It upgrades f.SchemaVersion to schema.Version4 if needed.
// Either start or brewFormula must be provided, but not both.
func ServiceAdd(f *schema.GenvFile, name string, start, stop, restart, status []string, brewFormula, targetID string) error {
	if name == "" {
		return errors.New("service name must not be empty")
	}
	if len(start) == 0 && brewFormula == "" {
		return errors.New("either --start or --brew-formula is required")
	}
	if len(start) > 0 && brewFormula != "" {
		return errors.New("--start and --brew-formula are mutually exclusive")
	}
	if f.SchemaVersion == schema.Version8 {
		bundle, err := ActiveBundle(f, targetID)
		if err != nil {
			return err
		}
		if bundle.Services == nil {
			bundle.Services = make(map[string]*schema.Service)
		}
		svc := schema.Service{
			Start:       start,
			Stop:        stop,
			Restart:     restart,
			Status:      status,
			BrewFormula: brewFormula,
		}
		bundle.Services[name] = &svc
		return nil
	}
	if f.Services == nil {
		f.Services = make(map[string]schema.Service)
	}
	f.Services[name] = schema.Service{
		Start:       start,
		Stop:        stop,
		Restart:     restart,
		Status:      status,
		BrewFormula: brewFormula,
	}
	// Raise schema to at least v4 now that a services block is present, without
	// downgrading a file that already declares a newer version.
	f.SchemaVersion = schema.AtLeastVersion(f.SchemaVersion, schema.Version4)
	return nil
}

// ServiceRemove removes the service from f's services block.
// Returns ErrServiceNotFound when name is absent.
func ServiceRemove(f *schema.GenvFile, name, targetID string) error {
	if f.SchemaVersion == schema.Version8 {
		bundle, err := ActiveBundle(f, targetID)
		if err != nil {
			return err
		}
		if bundle.Services != nil {
			if _, ok := bundle.Services[name]; ok {
				if defaultServiceExists(f.Defaults, name) {
					bundle.Services[name] = nil
				} else {
					delete(bundle.Services, name)
				}
				return nil
			}
		}
		if defaultServiceExists(f.Defaults, name) {
			if bundle.Services == nil {
				bundle.Services = make(map[string]*schema.Service)
			}
			bundle.Services[name] = nil
			return nil
		}
		return fmt.Errorf("%w: %q\nTip: run 'genv service list' to see declared services", ErrServiceNotFound, name)
	}
	if _, ok := f.Services[name]; !ok {
		return fmt.Errorf("%w: %q\nTip: run 'genv service list' to see declared services", ErrServiceNotFound, name)
	}
	delete(f.Services, name)
	return nil
}

// ServiceList writes a tabular listing of all declared services to w.
func ServiceList(f *schema.GenvFile, w io.Writer) {
	if len(f.Services) == 0 {
		_, _ = fmt.Fprintln(w, "no services declared.")
		return
	}

	names := make([]string, 0, len(f.Services))
	for name := range f.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tSTART\tSTOP\tSTATUS")
	for _, name := range names {
		svc := f.Services[name]
		stop := strings.Join(svc.Stop, " ")
		if stop == "" {
			stop = "—"
		}
		status := strings.Join(svc.Status, " ")
		if status == "" {
			status = "—"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, strings.Join(svc.Start, " "), stop, status)
	}
	_ = tw.Flush()
}
