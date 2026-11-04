package nomad

import (
	"context"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// AgentTarget is the agent whose log is followed. With neither id set it is
// the agent the client talks to. ServerID is the name of a server member.
type AgentTarget struct {
	NodeID   string
	ServerID string
}

// AgentLevels are the levels of an agent log, the quietest first.
var AgentLevels = []string{"error", "warn", "info", "debug", "trace"}

// AgentLogs follows what an agent logs at a level and above. The past lines
// of its log file come first, where the cluster has them, then new lines. The
// caller closes the stream when it stops reading. A failure arrives on its
// Err after the lines before it and before Lines closes. The context is not
// used because the stream outlives the call.
func (c *Client) AgentLogs(_ context.Context, target AgentTarget, level string) *LogStream {
	cancel := make(chan struct{})
	lines := make(chan string)
	errs := make(chan error, 1)

	go c.followAgent(target, level, lines, errs, cancel)

	return &LogStream{Lines: lines, Err: errs, cancel: cancel}
}

func (c *Client) followAgent(target AgentTarget, level string, lines chan<- string, errs chan<- error, cancel chan struct{}) {
	defer close(lines)

	if past := c.agentPast(target, level, cancel); past != "" && !sendText(lines, past, cancel) {
		return
	}

	select {
	case <-cancel:
		return
	default:
	}

	frames, failed := c.api.Agent().Monitor(cancel, c.agentQuery(target, map[string]string{"log_level": level}))

	followFrames(frames, failed, notCut, nil, lines, errs, cancel)
}

// agentPast reads the log file of the agent and keeps its last MiB at the
// level. A cluster that cannot give it (before Nomad 1.10.4, an agent with no
// log file, a token without the right) has none, and that is no error, nor is
// a read that breaks partway. A server asked for its past by name would
// answer with its live log, so it is not asked.
func (c *Client) agentPast(target AgentTarget, level string, cancel chan struct{}) string {
	if target.ServerID != "" {
		return ""
	}

	frames, errs := c.api.Agent().MonitorExport(cancel, c.agentQuery(target, map[string]string{"on_disk": "true"}))
	if frames == nil {
		return ""
	}

	past := tail{limit: fileTail}

	for open := true; open; {
		var frame *api.StreamFrame

		select {
		case frame, open = <-frames:
			if frame != nil {
				_, _ = past.Write(frame.Data)
			}

		case <-cancel:
			go drain(frames)

			return ""
		}
	}

	// The frames close before the error that says why they did: the past is
	// kept only when its read ended with io.EOF.
	select {
	case err := <-errs:
		if !errors.Is(err, io.EOF) {
			return ""
		}

	case <-cancel:
		return ""
	}

	return atLevel(string(past.text()), level)
}

// agentQuery is the query for the agent of the target, with the parameters.
func (c *Client) agentQuery(target AgentTarget, params map[string]string) *api.QueryOptions {
	q := c.query(context.Background(), "")

	q.Params = params

	if target.NodeID != "" {
		q.Params["node_id"] = target.NodeID
	}

	if target.ServerID != "" {
		q.Params["server_id"] = target.ServerID
	}

	return q
}

var (
	textLevel = regexp.MustCompile(`^(?:\S+\s+)?\[(TRACE|DEBUG|INFO|WARN|ERROR)\]`)
	jsonLevel = regexp.MustCompile(`"@level"\s*:\s*"(trace|debug|info|warn|error)"`)
)

// atLevel keeps the lines of the text at the level and above, each ending in
// a line break. A line with no level of its own goes with the line before it.
func atLevel(text, level string) string {
	limit := slices.Index(AgentLevels, level)
	keep := true

	var kept strings.Builder

	for line := range strings.Lines(text) {
		if rank := lineLevel(line); rank >= 0 {
			keep = rank <= limit
		}

		if keep {
			kept.WriteString(strings.TrimSuffix(line, "\n") + "\n")
		}
	}

	return kept.String()
}

// lineLevel is the rank of the level a text or JSON line names in
// AgentLevels, or -1.
func lineLevel(line string) int {
	match := textLevel.FindStringSubmatch(line)
	if match == nil && strings.HasPrefix(line, "{") {
		match = jsonLevel.FindStringSubmatch(line)
	}

	if match == nil {
		return -1
	}

	return slices.Index(AgentLevels, strings.ToLower(match[1]))
}

// notCut says the first frame does not begin inside a line: the live log is
// read from its start.
func notCut(*api.StreamFrame) bool { return false }

// sendText sends the text, or returns false when cancel comes first.
func sendText(lines chan<- string, text string, cancel <-chan struct{}) bool {
	select {
	case lines <- text:
		return true
	case <-cancel:
		return false
	}
}
