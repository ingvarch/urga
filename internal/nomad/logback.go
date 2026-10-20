package nomad

import (
	"bytes"
	"context"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// logStep is how far back one read goes, as much as a big file opens with.
const logStep = 1 << 20

// logDir is the folder of an allocation its logs are kept in.
const logDir = "alloc/logs"

// LogMark is where a byte of a log is: the file that holds it and its offset
// there. The cluster turns a log over into numbered files by size, so
// alloc/logs/server.stdout.3 is the fourth of them.
type LogMark struct {
	File   string
	Offset int64
}

// AtStart says the mark is the first byte of the first file of a log.
func (m LogMark) AtStart() bool {
	_, number, ok := splitLogFile(m.File)

	return ok && number == 0 && m.Offset == 0
}

// LogPart is a part of a log read from before a mark. From is where its
// first line begins, Before how much of the log the cluster keeps before
// that (0: nothing readable), and Size how big the log is.
type LogPart struct {
	Lines  []string
	From   LogMark
	Before int64
	Size   int64
}

// splitLogFile splits the name of a file of a log into what the files of that
// log share and the number of this one.
func splitLogFile(name string) (shared string, number int, ok bool) {
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 {
		return "", 0, false
	}

	number, err := strconv.Atoi(name[dot+1:])

	return name[:dot+1], number, err == nil && number >= 0
}

// logPiece is the bytes read from one file of a log, from an offset.
type logPiece struct {
	file string
	from int64
	data []byte
}

// LogBefore reads the log of a task from the step before a mark, which is
// where a line begins, and gives back the last limit lines of it. A log
// grows while it is read from its end, so this reads the files by offset.
func (c *Client) LogBefore(ctx context.Context, namespace, allocID string, at LogMark, limit int) (LogPart, error) {
	alloc := &api.Allocation{ID: allocID}

	// The client joins the path of a frame with its own separator, a
	// backslash on Windows.
	name := at.File[strings.LastIndexAny(at.File, `/\`)+1:]

	sizes, err := c.logSizes(ctx, namespace, alloc, name)
	if err != nil {
		return LogPart{}, err
	}

	part := LogPart{From: at, Size: sumOf(sizes)}

	shared, number, _ := splitLogFile(path.Join(logDir, name))
	if _, listed := sizes[number]; !listed {
		return part, nil
	}

	pieces, err := c.readBack(ctx, namespace, alloc, shared, number, at.Offset, sizes)
	if err != nil || len(pieces) == 0 {
		return part, err
	}

	return joined(pieces, sizes, limit, part), nil
}

// logSizes are the sizes of the files of the log the file of that name
// belongs to, by their number.
func (c *Client) logSizes(ctx context.Context, namespace string, alloc *api.Allocation, name string) (map[int]int64, error) {
	infos, _, err := c.api.AllocFS().List(alloc, logDir, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}

	shared, _, _ := splitLogFile(name)
	sizes := map[int]int64{}

	for _, info := range infos {
		if rest, ok := strings.CutPrefix(info.Name, shared); ok {
			if number, err := strconv.Atoi(rest); err == nil {
				sizes[number] = info.Size
			}
		}
	}

	return sizes, nil
}

// readBack reads up to logStep bytes back from offset in file number of the
// log whose files share a name, file by file down the numbers while the next
// one is listed. The pieces are in the order of the log.
func (c *Client) readBack(ctx context.Context, namespace string, alloc *api.Allocation, shared string, number int, offset int64, sizes map[int]int64) ([]logPiece, error) {
	var pieces []logPiece

	for remaining := int64(logStep); ; {
		take := min(remaining, offset)

		if take > 0 {
			file := shared + strconv.Itoa(number)

			data, err := c.readAt(ctx, namespace, alloc, file, offset-take, take)
			if err != nil {
				return nil, err
			}

			pieces = append([]logPiece{{file: file, from: offset - take, data: data}}, pieces...)
			remaining -= take
		}

		size, lower := sizes[number-1]
		if remaining <= 0 || !lower {
			return pieces, nil
		}

		number, offset = number-1, size
	}
}

// readAt reads count bytes of a file from an offset.
func (c *Client) readAt(ctx context.Context, namespace string, alloc *api.Allocation, file string, offset, count int64) ([]byte, error) {
	body, err := c.api.AllocFS().ReadAt(alloc, file, offset, count, c.query(ctx, namespace))
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	return io.ReadAll(body)
}

// joined makes the part out of what was read: its first line dropped when
// the read began inside a line, and only the last limit lines kept. part
// holds the mark it was asked for and the size of the log.
func joined(pieces []logPiece, sizes map[int]int64, limit int, part LogPart) LogPart {
	var data []byte
	for _, piece := range pieces {
		data = append(data, piece.data...)
	}

	begin := LogMark{File: pieces[0].file, Offset: pieces[0].from}
	_, first, _ := splitLogFile(begin.File)
	oldest := begin.Offset == 0 && !hasFile(sizes, first-1)
	cut := cutLength(data, begin, oldest)

	// A read that reached the oldest file left cannot read the cut line any
	// better later.
	unreadable := 0
	if oldest {
		unreadable = cut
	}

	lines, skipped := lastLines(data[cut:], limit)
	end := part.From
	part.Lines = lines
	part.From = markAt(pieces, cut+skipped, end)

	_, number, _ := splitLogFile(part.From.File)
	part.Before = part.From.Offset - int64(unreadable)

	for other, size := range sizes {
		if other < number {
			part.Before += size
		}
	}

	return part
}

// hasFile says the log has a file with the number.
func hasFile(sizes map[int]int64, number int) bool {
	_, ok := sizes[number]

	return ok
}

// cutLength is how many bytes of the start of a read are a line whose start
// was not read: none when it began where the log does, or has no line break
// before its last byte. A break at the last byte counts when the read reached
// the oldest file left, where it cannot have been a longer line.
func cutLength(data []byte, begin LogMark, oldest bool) int {
	if begin.AtStart() || len(data) == 0 {
		return 0
	}

	searched := data[:len(data)-1]
	if oldest {
		searched = data
	}

	if at := bytes.IndexByte(searched, '\n'); at >= 0 {
		return at + 1
	}

	return 0
}

// lastLines are the last limit lines of data, and how many bytes the lines
// left out take.
func lastLines(data []byte, limit int) (lines []string, skipped int) {
	if len(data) == 0 {
		return nil, 0
	}

	lines = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")

	for len(lines) > max(limit, 0) {
		skipped += len(lines[0]) + 1
		lines = lines[1:]
	}

	return lines, min(skipped, len(data))
}

// markAt is where the byte at index is, in the order of the pieces; end when
// it is past them.
func markAt(pieces []logPiece, index int, end LogMark) LogMark {
	for _, piece := range pieces {
		if index < len(piece.data) {
			return LogMark{File: piece.file, Offset: piece.from + int64(index)}
		}

		index -= len(piece.data)
	}

	return end
}

// sumOf is the sum of the sizes.
func sumOf(sizes map[int]int64) int64 {
	var total int64
	for _, size := range sizes {
		total += size
	}

	return total
}
