package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"
)

func TestHeader(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{
		address:      "https://nomad.example.com",
		version:      "v0.1.0-dev",
		nomadVersion: "1.11.1",
	}, 80)

	rows := lines(out)

	r.Contains(rows[0], "Address:")
	r.Contains(rows[0], "https://nomad.example.com")
	r.Contains(rows[3], "Urga Rev:")
	r.Contains(rows[3], "v0.1.0-dev")
	r.Contains(rows[4], "Nomad Rev:")
	r.Contains(rows[4], "1.11.1")

	for i, row := range rows {
		r.LessOrEqual(ansi.StringWidth(row), 80, "line %d", i)
	}
}

func TestHeader_LongAddressIsEaten(t *testing.T) {
	r := require.New(t)

	long := "https://" + strings.Repeat("nomad-cluster.", 10) + "example.com"

	out := renderHeader(header{address: long}, 60)
	rows := lines(out)

	// A value that does not fit is cut. Wrapping it pushes the rest of the
	// header down and the screen jumps.
	r.Len(rows, headerHeight)
	r.LessOrEqual(ansi.StringWidth(rows[0]), 60)
	r.Contains(rows[0], "…")
}

func TestHeader_ShowsTheKeysOfTheScreen(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{
		address: "https://nomad.example.com",
		hints: []hint{
			{Key: "<enter>", Description: "Allocations"},
			{Key: "<r>", Description: "Restart"},
		},
	}, 80)

	rows := lines(out)

	// The keys of the open resource sit next to the cluster info.
	r.Contains(rows[0], "<enter>")
	r.Contains(rows[0], "Allocations")
	r.Contains(rows[1], "<r>")
	r.Contains(rows[1], "Restart")
}

func TestHeader_Height(t *testing.T) {
	r := require.New(t)

	// The header is the same height whatever it holds, so the list below it
	// does not move.
	r.Len(lines(renderHeader(header{}, 80)), headerHeight)

	many := []hint{{Key: "<a>"}, {Key: "<b>"}, {Key: "<c>"}, {Key: "<d>"}, {Key: "<e>"}}
	r.Len(lines(renderHeader(header{hints: many}, 80)), headerHeight)
}

func TestHeader_LogoOnTheRight(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{address: "https://nomad.example.com", version: "v0.1.0-dev"}, 140)
	rows := lines(out)

	// The art sits at the right edge, the cluster info keeps the left.
	r.Contains(rows[0], "@@@  @@@")
	r.True(strings.HasPrefix(rows[0], "Address:"))

	for i, row := range rows {
		r.Equal(140, ansi.StringWidth(row), "line %d", i)
	}
}

func TestHeader_WithoutRoomForTheLogo(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{address: "https://nomad.example.com"}, 80)

	// No art at all, and no piece of it either.
	r.NotContains(out, "@")
	r.Len(lines(out), headerHeight)
}

func TestHeader_LeavesTheNamespaceToTheKeys(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{
		address:    "https://nomad.example.com",
		namespaces: []namespaceKey{{Key: "<1>", Name: "production", Active: true}},
	}, 140)

	// The namespace in use is the lit up key, saying it twice in one header
	// is a line wasted.
	r.NotContains(plain(out), "Namespace:")
	r.Contains(plain(out), "<1> production")
}

func TestHeader_IsAsTallAsWhatItHolds(t *testing.T) {
	r := require.New(t)

	// The art and the cluster info both fit, whichever of them is taller.
	r.GreaterOrEqual(headerHeight, len(logo))
	r.GreaterOrEqual(headerHeight, infoRows)
}

func TestHeader_ShowsWhatTheClusterIsUsing(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{usage: "15%", memory: "31%"}, 140)
	rows := lines(out)

	r.Contains(rows[5], "CPU:")
	r.Contains(rows[5], "15%")
	r.Contains(rows[6], "MEM:")
	r.Contains(rows[6], "31%")
}

func TestHeader_BeforeTheClusterAnswers(t *testing.T) {
	r := require.New(t)

	// Nothing is known yet, and the header says that rather than zero.
	out := renderHeader(header{}, 140)

	r.Contains(lines(out)[5], "n/a")
	r.Contains(lines(out)[4], "n/a")
}

func TestHeader_SaysTheRegionAndTheDatacenterUnderTheAddress(t *testing.T) {
	r := require.New(t)

	out := renderHeader(header{
		address:    "https://nomad.example.com",
		region:     "eu",
		datacenter: "dc1",
		version:    "v0.1.0-dev",
	}, 80)

	rows := lines(out)

	r.Contains(rows[0], "Address:")
	r.Contains(rows[1], "Region:")
	r.Contains(rows[1], "eu")
	r.Contains(rows[2], "DC:")
	r.Contains(rows[2], "dc1")
	r.Contains(rows[3], "Urga Rev:")
}

func TestHeader_BeforeARegionIsKnown(t *testing.T) {
	r := require.New(t)

	rows := lines(renderHeader(header{}, 80))

	// The region is the agent's until it has said which one that is; no
	// datacenter chosen is every one of them.
	r.Contains(rows[1], "n/a")
	r.Contains(rows[2], "all")
}

// jobKeys are the keys of the job list as the header shows them, more than
// one column of them.
func jobKeys() []hint {
	keys := []hint{}
	for _, k := range jobsKeys {
		keys = append(keys, keyHint{press: k.press, label: k.label}.hint())
	}

	return keys
}

func TestHeader_TheArtGivesWayToTheKeys(t *testing.T) {
	r := require.New(t)

	out := plain(renderHeader(header{
		address:    "http://127.0.0.1:14646",
		version:    "v0.2.0",
		namespaces: []namespaceKey{{Key: "0", Name: "all", Active: true}, {Key: "1", Name: "default"}},
		hints:      jobKeys(),
	}, 116))

	// A key that is not in the header is one nobody learns about. The art
	// is only art: when the keys need its place, they get it.
	for _, h := range jobKeys() {
		r.Contains(out, h.Description)
	}

	r.NotContains(out, "@@@")
}

func TestHeader_TheArtStaysWhenTheKeysFit(t *testing.T) {
	r := require.New(t)

	out := plain(renderHeader(header{
		address: "http://127.0.0.1:14646",
		version: "v0.2.0",
		hints:   jobKeys()[:3],
	}, 116))

	r.Contains(out, "@@@")
	r.Contains(out, "Mark All")
}

func TestHeader_ANamedCluster(t *testing.T) {
	r := require.New(t)

	out := plain(renderHeader(header{cluster: "prod", address: "https://nomad.prod:4646", readOnly: true}, 120))
	rows := lines(out)

	// The name first: it is what tells prod from dev at a glance.
	r.Contains(rows[0], "Cluster:")
	r.Contains(rows[0], "prod  https://nomad.prod:4646 read-only")
	r.NotContains(out, "Address:")
}

func TestHeader_ShowsTheClusterOfTheSession(t *testing.T) {
	r := require.New(t)

	m := New(&fakeClient{}, Options{Cluster: "prod", Version: "v-test"})
	m, _ = m.update(sizeMsg())

	// The address gives way to the name when the column is short of room.
	r.Contains(plain(m.render()), "Cluster:   prod  https://nomad.example")
}
