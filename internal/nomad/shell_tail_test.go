package nomad

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTail_HoldsAtMostTwiceTheLimitHoweverMuchIsWritten(t *testing.T) {
	r := require.New(t)

	var printed strings.Builder
	for i := range 600_000 {
		fmt.Fprintf(&printed, "%09d\n", i)
	}

	all := printed.String()
	printedTo := tail{limit: outputTail}

	for chunk := range slices.Chunk([]byte(all), 1000) {
		n, err := printedTo.Write(chunk)
		r.NoError(err)
		r.Equal(len(chunk), n)
		r.LessOrEqual(len(printedTo.kept), 2*printedTo.limit)
	}

	// 1 MiB holds 104857 lines of 10 bytes and the end of one more.
	r.Equal(all[len(all)-104857*10:], string(printedTo.text()))
	r.Equal(int64(len(all)), printedTo.size)
}

func TestTail_OneWriteLargerThanTheLimitKeepsOnlyItsEnd(t *testing.T) {
	r := require.New(t)

	var printed strings.Builder
	for i := range 100 {
		fmt.Fprintf(&printed, "%04d\n", i)
	}

	all := printed.String()
	printedTo := tail{limit: 22}

	n, err := printedTo.Write([]byte(all))
	r.NoError(err)
	r.Equal(len(all), n)

	// What is held is bounded by the limit, not by the size of the write.
	r.LessOrEqual(cap(printedTo.kept), 2*printedTo.limit)

	// The last 22 bytes start within 0095, so the lines from 0096 on are kept.
	r.Equal("0096\n0097\n0098\n0099\n", string(printedTo.text()))
	r.Equal(int64(len(all)), printedTo.size)
}

func TestTail_WithNoLineBreakItStartsAtACharacter(t *testing.T) {
	// The euro sign takes 3 bytes, the 3rd to the 5th.
	for limit, want := range map[int]string{4: "cdef", 5: "cdef", 6: "cdef", 7: "€cdef"} {
		printedTo := tail{limit: limit}

		_, err := printedTo.Write([]byte("ab€cdef"))
		require.NoError(t, err)

		require.Equal(t, want, string(printedTo.text()), "limit %d", limit)
	}

	// The face takes 4 bytes, the 3rd to the 6th.
	for limit, want := range map[int]string{5: "cdef", 6: "cdef", 7: "cdef", 8: "\U0001F600cdef"} {
		printedTo := tail{limit: limit}

		_, err := printedTo.Write([]byte("ab\U0001F600cdef"))
		require.NoError(t, err)

		require.Equal(t, want, string(printedTo.text()), "limit %d", limit)
	}
}
