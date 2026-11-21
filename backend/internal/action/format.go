package action

import (
	"fmt"
	"strings"
)

// FormatVerbose translates a compact Action into a verbose, human-readable string for logs.
func FormatVerbose(a Action) string {
	return FormatVerboseWithRegistry(DefaultRegistry(), a)
}

// FormatVerboseWithRegistry translates a compact Action using the given registry.
func FormatVerboseWithRegistry(r *Registry, a Action) string {
	def, ok := r.Lookup(a.Code)
	name := a.Code
	var args string
	if ok {
		name = def.Name
		if def.FormatArgs != nil {
			args = def.FormatArgs(&a)
		}
	}

	if args != "" {
		return fmt.Sprintf("[#%d] %s (%s)", a.Seq, name, args)
	}
	return fmt.Sprintf("[#%d] %s", a.Seq, name)
}

// FormatVerboseList formats a slice of compact actions as a multi-line human-readable summary.
func FormatVerboseList(actions []Action) string {
	if len(actions) == 0 {
		return "[]"
	}
	lines := make([]string, len(actions))
	for i, a := range actions {
		lines[i] = FormatVerbose(a)
	}
	return strings.Join(lines, "\n")
}

// ToVerboseMap converts a compact Action into a verbose, human-readable map
// compatible with structured logs and frontend WebSocket payloads.
func ToVerboseMap(a Action) map[string]interface{} {
	return ToVerboseMapWithRegistry(DefaultRegistry(), a)
}

// ToVerboseMapWithRegistry converts a compact Action into a verbose map using the given registry.
func ToVerboseMapWithRegistry(r *Registry, a Action) map[string]interface{} {
	code := r.ResolveCode(a.Code)
	name := r.ResolveName(a.Code)

	m := map[string]interface{}{
		"action":           name,
		"actionSequenceId": a.Seq,
	}

	if a.X != 0 || a.Y != 0 {
		m["coordinates"] = map[string]int{"x": a.X, "y": a.Y}
	}
	if a.Dur != 0 {
		m["duration"] = a.Dur
	}
	if a.Text != "" {
		m["inputString"] = a.Text
	}
	if a.Key != "" {
		if code == CodeKeyTap {
			m["keyTapString"] = a.Key
		} else {
			m["keyString"] = a.Key
		}
	}
	if len(a.Range) > 0 {
		m["actionsRange"] = a.Range
	}
	if a.Repeat != 0 {
		m["repeatTimes"] = a.Repeat
	}

	return m
}
