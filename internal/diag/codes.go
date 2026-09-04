package diag

import "sort"

// explain is the long-form help behind `kiln explain <code>`. Keeping it out
// of the message itself is deliberate: the message must be short enough to
// scan in a list, and the agent pulls the detail only when it needs it.
var explain = map[string]string{
	"K001": "Kiln indents with spaces. Replace the tab with two spaces per level.",
	"K002": "Indentation goes up two spaces at a time. A line indented by an odd\nnumber of spaces is ambiguous about which block it belongs to.",
	"K003": "This line is indented further than its parent allows. A block's children\nsit exactly one level (two spaces) inside it.",
	"K004": "A string opened with \" was never closed on this line. Kiln strings do not\nspan lines.",
	"K005": "This character has no meaning in Kiln. Check §expr for the operators the\nlanguage has.",
	"K010": "This word does not start any block Kiln knows. Run `kiln docs --section`\nfor the file kind you are writing to see the blocks it allows.",
	"K011": "This declaration is missing a block it requires — for example a route\nwithout a view, or an action without a do.",
	"K012": "Nesting is capped at six levels. Deep trees force an agent to match exact\nindentation to edit them. Split the inner part into its own route or block.",
	"K013": "Files are capped at 120 lines so one screen stays one read. Split this into\nanother route, action or schema file.",
	"K020": "No table by this name is declared in schema/. Check the spelling, or add\nthe table.",
	"K021": "This table has no such field. Fields come from the table's declaration in\nschema/ — add it there, or use one that exists.",
	"K022": "No action by this name is declared in actions/. A `do=` must name a real\naction so the click has somewhere to land.",
	"K023": "This link points at a path no route declares. Every internal link resolves\nat check time, so a dead link is a build error rather than a 404.",
	"K024": "Style values come from a closed set. There is no CSS and no class names, so\na token that is not in the set is always a mistake.",
	"K025": "No stdlib function by this name. Kiln has no user-defined functions; see\n`kiln docs --section stdlib` for the whole list.",
	"K030": "This expression has the wrong type for where it is used.",
	"K031": "The action declares this parameter in its `in` block but the caller does not\nsupply it. Every parameter is required.",
	"K032": "This argument is not declared in the action's `in` block. Add it there or\nremove it here.",
	"K040": "This action's `allow` rule reads session state that the calling route's\n`guard` does not establish, so the call can never succeed. Either widen the\nguard or narrow the rule.",
	"K042": "Two declarations share a name, or two routes share a path. Names are\nhow an agent addresses things, so they have to be unique.",
	"K050": "No view element by this name. The element vocabulary is closed — there is\nno CSS and no class names — so see `kiln docs --section view` for the set.",
	"K051": "This element takes a different number of positional arguments.",
	"K052": "This element does not accept that attribute.",
	"K053": "Something required was not supplied — an attribute the element needs, or a\nfield the table has no default for.",
	"K041": "A form field does not match any parameter the action declares.",
}

// Codes lists every known diagnostic code.
func Codes() []string {
	out := make([]string, 0, len(explain))
	for c := range explain {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Lookup returns the long-form help for a code.
func Lookup(code string) (string, bool) {
	s, ok := explain[code]
	return s, ok
}

// Suggest returns the candidates closest to word, for the "did you mean" line.
// A hallucinated name is the most common agent error, and naming the right one
// turns a guess-and-retry loop into a single edit.
func Suggest(word string, candidates []string) []string {
	type scored struct {
		name string
		dist int
	}
	var near []scored
	// A tight threshold on purpose. One edit for a short word, two for a longer
	// one, which catches a transposition like "titel" for "title" but refuses to
	// guess at a word that is simply wrong — a bad guess costs more than none.
	limit := 1
	if len(word) >= 5 {
		limit = 2
	}
	for _, c := range candidates {
		if d := distance(word, c); d <= limit {
			near = append(near, scored{c, d})
		}
	}
	sort.Slice(near, func(i, j int) bool {
		if near[i].dist != near[j].dist {
			return near[i].dist < near[j].dist
		}
		return near[i].name < near[j].name
	})
	var out []string
	for i, n := range near {
		if i == 3 {
			break
		}
		out = append(out, n.name)
	}
	return out
}

// distance is Damerau-Levenshtein edit distance, restricted to adjacent
// transpositions. Plain Levenshtein scores a transposition as two edits, which
// would miss "Tsak" for "Task" — the single most common typo there is.
func distance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	// Three rolling rows: two back is what a transposition reaches for.
	prev2 := make([]int, len(br)+1)
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				cur[j] = min(cur[j], prev2[j-2]+1)
			}
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(br)]
}
