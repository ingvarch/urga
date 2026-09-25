package ui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilter_KeepsWhatSaysIt(t *testing.T) {
	r := require.New(t)

	match := matcher("web")

	r.True(match("web production running"))
	r.False(match("cron production dead"))
}

func TestFilter_InverseLeavesTheRest(t *testing.T) {
	r := require.New(t)

	// `!web` keeps everything that does not contain web.
	match := matcher("!web")

	r.False(match("web production running"))
	r.True(match("cron production dead"))
}

func TestFilter_FuzzyTakesTheLettersInOrder(t *testing.T) {
	r := require.New(t)

	// `-f pbb` matches the letters in that order, with anything between
	// them.
	match := matcher("-f pbb")

	r.True(match("pelmeni_buh_bot"))
	r.True(match("PELMENI_BUH_BOT"))
	r.False(match("bbp"))
	r.False(match("nginx"))
}

func TestFilter_AnEmptyOneKeepsEverything(t *testing.T) {
	r := require.New(t)

	for _, filter := range []string{"", "!", "-f "} {
		r.True(matcher(filter)("anything at all"), "%q", filter)
	}
}

func TestFilter_NothingIsLitUpForOneThatExcludes(t *testing.T) {
	r := require.New(t)

	// A line is kept because it does not contain the word: there is nothing
	// in it to highlight.
	r.Nil(matchIn("cron production", "!web"))

	// And the letters of a fuzzy filter are spread over the line, so
	// highlighting the first of them would be misleading.
	r.Nil(matchIn("pelmeni_buh_bot", "-f pbb"))
}

func TestFilter_InverseOnTheScreen(t *testing.T) {
	r := require.New(t)

	m := loadedModel(t)

	m, _ = m.update(key('/'))
	m = typeIn(m, "!cron")
	m, _ = m.update(enter())

	out := plain(m.render())
	r.Contains(out, "web")
	r.NotContains(out, "cron")
}
