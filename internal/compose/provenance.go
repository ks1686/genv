package compose

import (
	"sort"
	"strings"
)

// Origin records where one resource declaration came from. It deliberately
// carries no declaration content: an Origin is safe to print in diagnostics,
// `genv explain`, and JSON output, which is what makes module ownership
// explainable without leaking env values or hook bodies.
type Origin struct {
	// Document is the file the declaration lives in: "genv.json" for the root
	// spec, or the registered module path for a module document.
	Document string `json:"document"`
	// Field is the JSON field path inside that document, e.g.
	// "targets.macos.services.searxng".
	Field string `json:"field"`
	// Module is the module name for module declarations, empty for the root.
	Module string `json:"module,omitempty"`
	// Target is the OS target bucket the declaration came from ("macos"), or
	// "defaults" for a contributor's defaults bundle.
	Target string `json:"target,omitempty"`
}

func (o Origin) String() string {
	label := o.Document
	if o.Module != "" {
		label += " (module " + o.Module + ")"
	}
	if o.Target != "" {
		label += " [" + o.Target + "]"
	}
	return label + " " + o.Field
}

// Provenance indexes every owner of every composed identity. An identity with
// more than one owner is a shared resource: it stays desired as long as any
// selected contributor declares it, which is what keeps a module deselection
// from removing a package another module still wants.
type Provenance struct {
	owners map[Identity][]Origin
}

func newProvenance() *Provenance {
	return &Provenance{owners: make(map[Identity][]Origin)}
}

// add records an owner for id, keeping declaration order and skipping duplicates
// so a resource coalesced across many documents lists each file exactly once.
func (p *Provenance) add(id Identity, origin Origin) {
	for _, existing := range p.owners[id] {
		if existing == origin {
			return
		}
	}
	p.owners[id] = append(p.owners[id], origin)
}

// Owners returns the declarations of id in declaration order.
func (p *Provenance) Owners(id Identity) []Origin {
	if p == nil {
		return nil
	}
	return p.owners[id]
}

// OwnersOf returns every owner of id that belongs to a module (not the root).
func (p *Provenance) OwnersOf(id Identity) []Origin {
	var out []Origin
	for _, o := range p.Owners(id) {
		if o.Module != "" {
			out = append(out, o)
		}
	}
	return out
}

// IsModuleOwned reports whether any module contributes id. Mutation commands
// use this to refuse editing a declaration that lives in a module file.
func (p *Provenance) IsModuleOwned(id Identity) bool {
	return len(p.OwnersOf(id)) > 0
}

// Modules lists every module that owns at least one resource, sorted.
func (p *Provenance) Modules() []string {
	seen := map[string]bool{}
	for _, origins := range p.owners {
		for _, o := range origins {
			if o.Module != "" {
				seen[o.Module] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Identities returns every identity in the index, sorted by kind then key.
func (p *Provenance) Identities() []Identity {
	out := make([]Identity, 0, len(p.owners))
	for id := range p.owners {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// String renders the index as "kind:key <- owner; owner" lines for diagnostics.
func (p *Provenance) String() string {
	var b strings.Builder
	for _, id := range p.Identities() {
		b.WriteString(id.String())
		b.WriteString(" <- ")
		b.WriteString(strings.Join(originStrings(p.Owners(id)), "; "))
		b.WriteByte('\n')
	}
	return b.String()
}

func originStrings(origins []Origin) []string {
	out := make([]string, len(origins))
	for i, o := range origins {
		out[i] = o.String()
	}
	return out
}
