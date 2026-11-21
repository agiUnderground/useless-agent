package action

import (
	"fmt"
	"strings"
	"sync"
)

// Compact action codes used in LLM JSON output.
const (
	CodeMouseMove            = "mm"
	CodeMouseMoveRelative    = "mr"
	CodeMouseClickLeft       = "cl"
	CodeMouseClickLeftDouble = "cd"
	CodeMouseClickRight      = "cr"
	CodeDragSmooth           = "dr"
	CodeScrollSmooth         = "sc"
	CodePrintString          = "tp"
	CodeKeyTap               = "kt"
	CodeKeyDown              = "kd"
	CodeKeyUp                = "ku"
	CodeNop                  = "w"
	CodeRepeat               = "rp"
	CodeStopIteration        = "st"
)

// Registry holds action definitions and supports lookup by either compact code
// or verbose human-readable name.
type Registry struct {
	mu     sync.RWMutex
	byCode map[string]Definition
	byName map[string]Definition
	order  []string
}

// NewRegistry creates an empty action registry.
func NewRegistry() *Registry {
	return &Registry{
		byCode: make(map[string]Definition),
		byName: make(map[string]Definition),
	}
}

// Register adds or replaces an action definition in the registry.
func (r *Registry) Register(def Definition) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byCode[def.Code]; !exists {
		r.order = append(r.order, def.Code)
	}
	r.byCode[def.Code] = def
	if def.Name != "" {
		r.byName[def.Name] = def
	}
}

// Lookup finds a definition by compact code first, then by verbose name.
func (r *Registry) Lookup(codeOrName string) (Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if def, ok := r.byCode[codeOrName]; ok {
		return def, true
	}
	if def, ok := r.byName[codeOrName]; ok {
		return def, true
	}
	return Definition{}, false
}

// ResolveName returns the verbose human-readable name for a compact code or name.
func (r *Registry) ResolveName(codeOrName string) string {
	if def, ok := r.Lookup(codeOrName); ok && def.Name != "" {
		return def.Name
	}
	return codeOrName
}

// ResolveCode returns the canonical compact code for a compact code or verbose name.
func (r *Registry) ResolveCode(codeOrName string) string {
	if def, ok := r.Lookup(codeOrName); ok && def.Code != "" {
		return def.Code
	}
	return codeOrName
}

// All returns all registered definitions in registration order.
func (r *Registry) All() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]Definition, 0, len(r.order))
	for _, code := range r.order {
		defs = append(defs, r.byCode[code])
	}
	return defs
}

// PromptSpec builds a compact, token-efficient action schema reference for the LLM prompt.
func (r *Registry) PromptSpec() string {
	defs := r.All()
	var b strings.Builder
	b.WriteString("Output a compact JSON array of actions (no comments, valid JSON). Keys: i=sequence id (1..N), a=action code, x/y=coords or scroll delta, d=wait seconds, t=text, k=key, r=[start,end] 1-based range, n=repeat count.\n")
	b.WriteString("Actions:\n")
	for _, def := range defs {
		fmt.Fprintf(&b, "- %s (%s): %s | ex: %s\n", def.Code, def.Name, def.Description, def.ExampleJSON)
	}
	b.WriteString(`Keys for kt/kd/ku: "backspace","delete","enter","tab","esc","escape","up","down","right","left","home","end","pageup","pagedown","lctrl","rctrl","lalt","ralt","lshift","rshift".`)
	return b.String()
}

var (
	defaultRegistryOnce sync.Once
	defaultRegistry     *Registry
)

// DefaultRegistry returns the shared registry populated with all built-in actions.
func DefaultRegistry() *Registry {
	defaultRegistryOnce.Do(func() {
		defaultRegistry = NewRegistry()
		registerBuiltinActions(defaultRegistry)
	})
	return defaultRegistry
}
