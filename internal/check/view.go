package check

import "sort"

// element is one view element's contract.
//
// The vocabulary is closed on purpose. There is no CSS and no class names, so
// an element or a style token that is not in these tables is always a mistake
// the checker can name rather than a silent visual bug.
type element struct {
	minArgs, maxArgs int
	// attrs are the names this element accepts beyond the common ones.
	attrs []string
	// required attrs must be present.
	required []string
	// actionArgs marks an element that binds an action, whose remaining
	// attributes are that action's arguments and are checked against it.
	actionArgs bool
	// container marks an element that may have children.
	container bool
}

// commonAttrs apply to every element, and are reserved on all of them: an
// action-binding element can never pass one as an argument. Element identity is
// deliberately absent — the runtime derives region ids for patching, so an
// author never names one and `id=` always means an action's own parameter.
var commonAttrs = []string{"gap", "pad", "align", "style"}

var elements = map[string]element{
	// layout
	"page": {attrs: []string{"title"}, container: true},
	"col":  {container: true},
	"row":  {container: true},
	"grid": {attrs: []string{"cols"}, container: true},
	"card": {container: true},
	"sep":  {},

	// content
	"head":  {minArgs: 2, maxArgs: 2},
	"text":  {minArgs: 1, maxArgs: 1},
	"rich":  {minArgs: 1, maxArgs: 1},
	"img":   {attrs: []string{"src", "alt"}, required: []string{"src", "alt"}},
	"badge": {minArgs: 1, maxArgs: 1},
	"empty": {minArgs: 1, maxArgs: 1},

	// interaction
	"link":   {minArgs: 1, maxArgs: 1, attrs: []string{"to"}, required: []string{"to"}},
	"button": {minArgs: 1, maxArgs: 1, attrs: []string{"do", "confirm"}, required: []string{"do"}, actionArgs: true},
	"check":  {attrs: []string{"do", "value"}, required: []string{"do", "value"}, actionArgs: true},
	"form":   {attrs: []string{"do"}, required: []string{"do"}, actionArgs: true, container: true},
	"input":  {minArgs: 2, maxArgs: 3, attrs: []string{"max", "label", "placeholder"}},
	"select": {minArgs: 1, maxArgs: 1, attrs: []string{"from", "label"}, required: []string{"from"}},
	"area":   {minArgs: 1, maxArgs: 1, attrs: []string{"rows", "label"}},
	"submit": {minArgs: 1, maxArgs: 1},
}

// styleTokens is the complete set of style values.
var styleTokens = []string{"plain", "quiet", "strong", "danger", "good", "warn"}

// alignTokens is the complete set of align values.
var alignTokens = []string{"start", "center", "end"}

// inputTypes are the types an input or area may collect.
var inputTypes = []string{"text", "int", "num", "bool", "at"}

// spacingMax is the largest gap or pad step.
const spacingMax = 6

// maxViewDepth is how far a view may nest below its page.
//
// Measured on the view tree rather than on file indentation, because
// route > view > page is fixed overhead every route pays before any layout
// begins — counting it made an ordinary card-in-a-list hit the cap. The limit
// has so far produced two false positives and no true ones, so if it fires
// again on a reasonable layout it should be deleted rather than raised a third
// time.
const maxViewDepth = 6

// ElementNames lists every element, for suggestions and for the reference test.
func ElementNames() []string {
	out := make([]string, 0, len(elements)+3)
	for n := range elements {
		out = append(out, n)
	}
	out = append(out, "each", "when", "else")
	sort.Strings(out)
	return out
}

// attrAllowed reports whether an element accepts an attribute by that name.
func (e element) attrAllowed(name string) bool {
	for _, a := range commonAttrs {
		if a == name {
			return true
		}
	}
	for _, a := range e.attrs {
		if a == name {
			return true
		}
	}
	return false
}

// knownAttrs lists everything this element accepts, for suggestions.
func (e element) knownAttrs() []string {
	out := append([]string{}, commonAttrs...)
	out = append(out, e.attrs...)
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
