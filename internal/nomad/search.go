package nomad

import (
	"cmp"
	"context"
	"regexp"
	"slices"

	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/api/contexts"
)

// The kinds of thing a search finds.
const (
	MatchJob        = "job"
	MatchGroup      = "group"
	MatchTask       = "task"
	MatchService    = "service"
	MatchImage      = "image"
	MatchCommand    = "command"
	MatchClass      = "class"
	MatchAlloc      = "alloc"
	MatchNode       = "node"
	MatchPool       = "pool"
	MatchNamespace  = "namespace"
	MatchVariable   = "variable"
	MatchVolume     = "volume"
	MatchHostVolume = "host volume"
	MatchPlugin     = "plugin"
	MatchEval       = "eval"
	MatchDeployment = "deployment"
)

// Match is one thing a search found: what it is called, and what opens it.
type Match struct {
	Kind, Name string

	// Namespace is where it lives; empty for what lives in none, and for
	// what was found by its ID alone.
	Namespace string

	// ID opens it: the ID of a job, an allocation, a node or a volume, the
	// path of a variable.
	ID string

	// JobID, Group and Task are where a part of a job is.
	JobID, Group, Task string
}

// Found is what a search found, and the kinds the cluster cut short.
type Found struct {
	Matches   []Match
	Truncated []string
}

// scope is where the cluster says a match lives, from the namespace down to
// the parent of the match; a place it does not say is empty.
type scope []string

func (s scope) at(i int) string {
	if i < len(s) {
		return s[i]
	}

	return ""
}

// byName are the kinds the name search finds, in the order they are shown,
// and how the scope of each reads.
var byName = []struct {
	context contexts.Context
	kind    string
	read    func(name string, s scope) Match
}{
	{contexts.Jobs, MatchJob, func(name string, s scope) Match {
		return Match{Namespace: s.at(0), ID: cmp.Or(s.at(1), name)}
	}},
	{contexts.Groups, MatchGroup, func(name string, s scope) Match {
		return Match{Namespace: s.at(0), JobID: s.at(1), Group: name}
	}},
	{contexts.Tasks, MatchTask, func(name string, s scope) Match {
		return Match{Namespace: s.at(0), JobID: s.at(1), Group: s.at(2), Task: name}
	}},
	{contexts.Services, MatchService, inJob},
	{contexts.Images, MatchImage, inJob},
	{contexts.Commands, MatchCommand, inJob},
	{contexts.Allocs, MatchAlloc, inNamespace},
	{contexts.Nodes, MatchNode, func(_ string, s scope) Match { return Match{ID: s.at(0)} }},
	{contexts.Classes, MatchClass, func(string, scope) Match { return Match{} }},
	{contexts.NodePools, MatchPool, named},
	{contexts.Namespaces, MatchNamespace, named},
	{contexts.Variables, MatchVariable, inNamespace},
	{contexts.Volumes, MatchVolume, inNamespace},
	{contexts.HostVolumes, MatchHostVolume, inNamespace},
	{contexts.Plugins, MatchPlugin, named},
}

// inJob is a part of a job: where in the job it is, down to its task.
func inJob(_ string, s scope) Match {
	return Match{Namespace: s.at(0), JobID: s.at(1), Group: s.at(2), Task: s.at(3)}
}

// inNamespace is what lives in a namespace under an ID of its own.
func inNamespace(name string, s scope) Match {
	return Match{Namespace: s.at(0), ID: cmp.Or(s.at(1), name)}
}

// named is what its name opens.
func named(name string, _ scope) Match { return Match{ID: name} }

// byID are the kinds whose ID is a UUID the whole cluster shares: the search
// by ID finds them wherever they live. What has a name is found by it.
var byID = []struct {
	context contexts.Context
	kind    string
}{
	{contexts.Allocs, MatchAlloc},
	{contexts.Evals, MatchEval},
	{contexts.Deployments, MatchDeployment},
	{contexts.Nodes, MatchNode},
}

// idPrefix is text that can begin an ID.
var idPrefix = regexp.MustCompile(`^[0-9a-f-]+$`)

// Find searches the whole cluster for text: names, the parts of jobs, and,
// for text that can begin an ID, the IDs of what has one.
func (c *Client) Find(ctx context.Context, text string) (Found, error) {
	q := c.query(ctx, AllNamespaces)

	fuzzy, _, err := c.api.Search().FuzzySearch(text, contexts.All, q)
	if err != nil {
		return Found{}, err
	}

	found := Found{}

	for _, k := range byName {
		for _, m := range fuzzy.Matches[k.context] {
			match := k.read(m.ID, m.Scope)
			match.Kind, match.Name = k.kind, m.ID
			found.Matches = append(found.Matches, match)
		}

		found.cut(k.kind, fuzzy.Truncations[k.context])
	}

	if !idPrefix.MatchString(text) {
		return found, nil
	}

	prefix, _, err := c.api.Search().PrefixSearch(text, contexts.All, q)
	if err != nil {
		return Found{}, err
	}

	found.addIDs(prefix)

	return found, nil
}

// addIDs adds what the search by ID found and the name search did not.
func (f *Found) addIDs(prefix *api.SearchResponse) {
	for _, k := range byID {
		for _, id := range prefix.Matches[k.context] {
			if !f.has(k.kind, id) {
				f.Matches = append(f.Matches, Match{Kind: k.kind, Name: id, ID: id})
			}
		}

		f.cut(k.kind, prefix.Truncations[k.context])
	}
}

func (f *Found) has(kind, id string) bool {
	return slices.ContainsFunc(f.Matches, func(m Match) bool { return m.Kind == kind && m.ID == id })
}

// cut notes a kind the cluster cut short, once.
func (f *Found) cut(kind string, truncated bool) {
	if truncated && !slices.Contains(f.Truncated, kind) {
		f.Truncated = append(f.Truncated, kind)
	}
}
