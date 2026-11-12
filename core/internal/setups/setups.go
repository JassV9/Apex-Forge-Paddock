// Package setups owns a car's setup catalog and its setup versions: validating values against
// the catalog, diffing versions, hashing them and writing them as setup files.
//
// The validation rules are ported from Apex Forge v0 `core/reply.lua` and `core/catalog.lua`
// (their tests are still run in agent/tests): a value must be a known, changeable item, inside
// its range, and on the item's step grid.
package setups

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Parameter is one item of a car's setup menu.
type Parameter struct {
	Name     string
	Label    string
	Min      float64
	Max      float64
	Step     float64
	Excluded bool // power unit, display and read-only items: never changed by an experiment
}

// Catalog is a car's setup menu by item name.
type Catalog map[string]Parameter

// Values maps item names to raw values, as the game's spinners hold them.
type Values map[string]float64

// Validation errors. Use errors.Is; the wrapped message names the item.
var (
	ErrUnknownItem  = errors.New("not a setup item on this car")
	ErrExcludedItem = errors.New("item is excluded from experiments")
	ErrOutOfRange   = errors.New("value is outside the item's range")
	ErrOffGrid      = errors.New("value is not on the item's step grid")
)

const gridTolerance = 1e-6

// onGrid reports whether value is a whole number of steps from origin.
func onGrid(value, origin, step float64) bool {
	steps := (value - origin) / step
	return math.Abs(steps-math.Round(steps)) < gridTolerance
}

// Validate checks every value against the catalog. A value is legal on the grid that starts
// at the item's minimum, or on the grid through the item's current value in `anchor`: the
// in-game spinner steps from the current value, which is not always on the minimum's grid.
// anchor may be nil.
func Validate(catalog Catalog, values, anchor Values) error {
	for _, name := range sortedNames(values) {
		value := values[name]
		item, known := catalog[name]
		if !known {
			return fmt.Errorf("%s: %w", name, ErrUnknownItem)
		}
		if item.Excluded {
			return fmt.Errorf("%s (%s): %w", name, item.Label, ErrExcludedItem)
		}
		if value < item.Min || value > item.Max {
			return fmt.Errorf("%s = %v, range %v..%v: %w", name, value, item.Min, item.Max, ErrOutOfRange)
		}
		current, anchored := anchor[name]
		if !onGrid(value, item.Min, item.Step) && (!anchored || !onGrid(value, current, item.Step)) {
			return fmt.Errorf("%s = %v, step %v: %w", name, value, item.Step, ErrOffGrid)
		}
	}
	return nil
}

// Step returns base with one item moved by a number of clicks, validated. This is the only
// way an experiment makes a variant, so a variant always differs from its baseline in exactly
// one item.
func Step(catalog Catalog, base Values, name string, clicks int) (Values, error) {
	if clicks == 0 {
		return nil, fmt.Errorf("%s: a change of 0 clicks is not a change", name)
	}
	item, known := catalog[name]
	if !known {
		return nil, fmt.Errorf("%s: %w", name, ErrUnknownItem)
	}
	current, present := base[name]
	if !present {
		return nil, fmt.Errorf("%s: the baseline setup has no value for it: %w", name, ErrUnknownItem)
	}
	changed := Values{name: current + float64(clicks)*item.Step}
	if err := Validate(catalog, changed, base); err != nil {
		return nil, err
	}
	variant := make(Values, len(base))
	for key, value := range base {
		variant[key] = value
	}
	variant[name] = changed[name]
	return variant, nil
}

// Change is one item that differs between two setups.
type Change struct {
	Name string
	From float64
	To   float64
}

// Diff lists the items whose values differ, by name. An item missing on one side is ignored:
// it cannot be compared.
func Diff(a, b Values) []Change {
	var changes []Change
	for _, name := range sortedNames(a) {
		other, present := b[name]
		if present && other != a[name] {
			changes = append(changes, Change{Name: name, From: a[name], To: other})
		}
	}
	return changes
}

// Hash is the content hash of a setup: the same values always give the same hash, whatever
// order they were given in.
func Hash(values Values) string {
	var text strings.Builder
	for _, name := range sortedNames(values) {
		text.WriteString(name)
		text.WriteByte('=')
		text.WriteString(strconv.FormatFloat(values[name], 'g', -1, 64))
		text.WriteByte(';')
	}
	sum := sha256.Sum256([]byte(text.String()))
	return hex.EncodeToString(sum[:8])
}

func sortedNames(values Values) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
