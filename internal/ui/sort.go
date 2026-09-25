package ui

import (
	"sort"
	"strconv"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// sortState is which column the list is ordered by, and which way round.
type sortState struct {
	column int
	desc   bool
}

// none is the order the cluster answered in.
const noColumn = -1

func newSortState() sortState { return sortState{column: noColumn} }

// by orders the list by a column. The same column again turns it around.
func (s sortState) by(column int) sortState {
	if s.column == column {
		return sortState{column: column, desc: !s.desc}
	}

	return sortState{column: column}
}

// marker is what the header of the sorted column carries.
func (s sortState) marker(column int) string {
	if column != s.column {
		return ""
	}

	if s.desc {
		return " ↓"
	}

	return " ↑"
}

// columnOfLetter is the first column whose title starts with a letter, which
// is how a column is picked with one key.
func columnOfLetter(titles []string, letter rune) (int, bool) {
	wanted := unicode.ToLower(letter)

	for i, title := range titles {
		if title == "" {
			continue
		}

		if unicode.ToLower([]rune(title)[0]) == wanted {
			return i, true
		}
	}

	return noColumn, false
}

// sortRows orders the rows and carries the index along, so that the cursor
// still points at the resource under it.
func sortRows(rows []tableRow, index []int, state sortState, titles []string) ([]tableRow, []int) {
	if state.column == noColumn || state.column >= len(titles) {
		return rows, index
	}

	byTime := holdsAges(rows, state.column)
	now := time.Now()

	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}

	sort.SliceStable(order, func(a, b int) bool {
		left, right := rows[order[a]], rows[order[b]]

		if state.desc {
			left, right = right, left
		}

		if byTime {
			return ageIn(left, state.column, now) < ageIn(right, state.column, now)
		}

		return naturalLess(cellAt(left, state.column), cellAt(right, state.column))
	})

	sortedRows := make([]tableRow, len(rows))
	sortedIndex := make([]int, len(index))

	for i, from := range order {
		sortedRows[i] = rows[from]

		if from < len(index) {
			sortedIndex[i] = index[from]
		}
	}

	return sortedRows, sortedIndex
}

func cellAt(row tableRow, column int) string {
	if column >= len(row.cells) {
		return ""
	}

	return ansi.Strip(row.cells[column])
}

// holdsAges says whether a column shows ages, which the rows know from how
// they were built.
func holdsAges(rows []tableRow, column int) bool {
	for _, row := range rows {
		if _, ok := row.ages[column]; ok {
			return true
		}
	}

	return false
}

// ageIn is how long ago the moment behind a cell was. A cell with no moment
// reads "-" or nothing, and counts as new.
func ageIn(row tableRow, column int, now time.Time) time.Duration {
	moment := row.ages[column]
	if moment.IsZero() {
		return 0
	}

	return now.Sub(moment)
}

// naturalLess compares the way a person reads: the digits in a value count as
// numbers, so 9 comes before 10.
func naturalLess(a, b string) bool {
	ai, bi := 0, 0

	for ai < len(a) && bi < len(b) {
		if isDigit(a[ai]) && isDigit(b[bi]) {
			aNum, aNext := number(a, ai)
			bNum, bNext := number(b, bi)

			if aNum != bNum {
				return aNum < bNum
			}

			ai, bi = aNext, bNext

			continue
		}

		left, right := lower(a[ai]), lower(b[bi])
		if left != right {
			return left < right
		}

		ai++
		bi++
	}

	return len(a)-ai < len(b)-bi
}

func number(s string, from int) (int, int) {
	to := from
	for to < len(s) && isDigit(s[to]) {
		to++
	}

	value, _ := strconv.Atoi(s[from:to])

	return value, to
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}

	return c
}
