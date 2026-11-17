package setups

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// Links are the car's linked setup items (docs/adr/0026-linked-setup-items.md): items the car
// itself keeps equal, so they are one setting. Each member of a group maps to every member of
// its group, sorted by name; an item that is not linked is not in the map.
type Links map[string][]string

// MinRecordedVersions is how many setups read back from the car it takes before two items
// are taken to be linked from what was recorded.
const MinRecordedVersions = 3

// DeclaredLinks are groups known without recording: a car's scripts that link items, seen in
// the game. Keyed by the car's game id.
var DeclaredLinks = map[string][][]string{
	// Its scripts keep the second "Rear Wing" item equal to the first (seen 2026-11-17).
	"rss_formula_hybrid_2026": {{"WING_1", "WING_9"}},
}

// DetectLinks finds the car's linked items. Two items are linked when both can be changed,
// they have the same label and the same range and step, and in every setup read back from the
// car (at least MinRecordedVersions) both have a value, the same one, and that value is not
// the same in all of them: two items that never moved say nothing about each other. A declared
// group counts whatever was recorded.
func DetectLinks(catalog Catalog, recorded []Values, declared [][]string) Links {
	links := Links{}
	for _, group := range declared {
		links.add(catalog, group)
	}
	if len(recorded) < MinRecordedVersions {
		return links
	}
	candidates := map[string][]string{}
	for _, name := range slices.Sorted(maps.Keys(catalog)) {
		item := catalog[name]
		signature, moved := history(name, recorded)
		if item.Excluded || !moved || links[name] != nil {
			continue
		}
		key := fmt.Sprintf("%s|%v|%v|%v|%s", item.Label, item.Min, item.Max, item.Step, signature)
		candidates[key] = append(candidates[key], name)
	}
	for _, group := range candidates {
		links.add(catalog, group)
	}
	return links
}

// add records a group of two or more known items.
func (l Links) add(catalog Catalog, group []string) {
	members := []string{}
	for _, name := range group {
		if _, known := catalog[name]; known && !slices.Contains(members, name) {
			members = append(members, name)
		}
	}
	if len(members) < 2 {
		return
	}
	slices.Sort(members)
	for _, name := range members {
		l[name] = members
	}
}

// history is the item's value in every recorded setup, as text, and whether it took more than
// one value. An item missing from a setup has no history.
func history(name string, recorded []Values) (string, bool) {
	var text strings.Builder
	moved := false
	for i, values := range recorded {
		value, present := values[name]
		if !present {
			return "", false
		}
		moved = moved || value != recorded[0][name]
		if i > 0 {
			text.WriteByte(',')
		}
		text.WriteString(strconv.FormatFloat(value, 'g', -1, 64))
	}
	return text.String(), moved
}

// Members are the items a change of name moves: its group, or the item alone.
func (l Links) Members(name string) []string {
	if group, linked := l[name]; linked {
		return group
	}
	return []string{name}
}

// Leader is the name a group goes by: its first member. An item that is not linked leads
// itself.
func (l Links) Leader(name string) string {
	return l.Members(name)[0]
}

// Label is how the ledger and the pages name the item: a group once, with its members.
func (l Links) Label(catalog Catalog, name string) string {
	return GroupLabel(catalog[name].Label, l.Members(name))
}

// GroupLabel names items changed together: "Rear Wing (WING_1 + WING_9)", or the label alone
// for a single item.
func GroupLabel(label string, members []string) string {
	if len(members) < 2 {
		return label
	}
	return fmt.Sprintf("%s (%s)", label, strings.Join(members, " + "))
}
