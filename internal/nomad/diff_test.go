package nomad

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/nomad/api"
	"github.com/stretchr/testify/require"
)

// editedWeb is a diff of web: the group runs more allocations, the task has
// a changed and a new environment variable and more CPU, a service was taken
// out and a template put in.
const editedWeb = `{
	"Type": "Edited",
	"ID": "web",
	"Fields": [{"Type": "Edited", "Name": "Priority", "Old": "50", "New": "70"}],
	"TaskGroups": [{
		"Type": "Edited",
		"Name": "web",
		"Fields": [
			{"Type": "Edited", "Name": "Count", "Old": "1", "New": "3"},
			{"Type": "None", "Name": "Name", "Old": "web", "New": "web"}
		],
		"Tasks": [{
			"Type": "Edited",
			"Name": "server",
			"Fields": [
				{"Type": "Edited", "Name": "Env[MESSAGE_RU]", "Old": "Утречка!?", "New": "Утречка?"},
				{"Type": "Added", "Name": "Env[MODE]", "New": "fast"}
			],
			"Objects": [
				{"Type": "Edited", "Name": "Resources", "Fields": [
					{"Type": "Edited", "Name": "CPU", "Old": "100", "New": "200"},
					{"Type": "None", "Name": "MemoryMB", "Old": "64", "New": "64"}
				]},
				{"Type": "Deleted", "Name": "Service", "Fields": [
					{"Type": "Deleted", "Name": "PortLabel", "Old": "http"}
				]},
				{"Type": "Added", "Name": "Template", "Fields": [
					{"Type": "Added", "Name": "DestPath", "New": "local/app.env"}
				]},
				{"Type": "None", "Name": "LogConfig"}
			]
		}]
	}]
}`

func TestHCLDiff(t *testing.T) {
	r := require.New(t)

	var diff api.JobDiff
	r.NoError(json.Unmarshal([]byte(editedWeb), &diff))

	// The way the job file reads, the way git diff shows a change to it:
	// the blocks that lead to a change, and every changed value as the line
	// it was and the line it is. What did not change is left out.
	r.Equal([]DiffLine{
		{Kind: DiffDeleted, Indent: 0, Text: "priority = 50"},
		{Kind: DiffAdded, Indent: 0, Text: "priority = 70"},
		{},
		{Kind: DiffContext, Indent: 0, Text: `group "web" {`},
		{Kind: DiffDeleted, Indent: 1, Text: "count = 1"},
		{Kind: DiffAdded, Indent: 1, Text: "count = 3"},
		{},
		{Kind: DiffContext, Indent: 1, Text: `task "server" {`},
		{Kind: DiffContext, Indent: 2, Text: "env {"},
		{Kind: DiffDeleted, Indent: 3, Text: `MESSAGE_RU = "Утречка!?"`},
		{Kind: DiffAdded, Indent: 3, Text: `MESSAGE_RU = "Утречка?"`},
		{Kind: DiffAdded, Indent: 3, Text: `MODE = "fast"`},
		{Kind: DiffContext, Indent: 2, Text: "}"},
		{},
		{Kind: DiffContext, Indent: 2, Text: "resources {"},
		{Kind: DiffDeleted, Indent: 3, Text: "cpu = 100"},
		{Kind: DiffAdded, Indent: 3, Text: "cpu = 200"},
		{Kind: DiffContext, Indent: 2, Text: "}"},
		{},
		{Kind: DiffDeleted, Indent: 2, Text: "service {"},
		{Kind: DiffDeleted, Indent: 3, Text: `port_label = "http"`},
		{Kind: DiffDeleted, Indent: 2, Text: "}"},
		{},
		{Kind: DiffAdded, Indent: 2, Text: "template {"},
		{Kind: DiffAdded, Indent: 3, Text: `dest_path = "local/app.env"`},
		{Kind: DiffAdded, Indent: 2, Text: "}"},
		{Kind: DiffContext, Indent: 1, Text: "}"},
		{Kind: DiffContext, Indent: 0, Text: "}"},
	}, hclDiff(&diff))
}

func TestHCLDiff_Names(t *testing.T) {
	r := require.New(t)

	// Fields are named the way a job file names them.
	r.Equal("port_label", hclName("PortLabel"))
	r.Equal("http_port", hclName("HTTPPort"))
	r.Equal("disk_mb", hclName("DiskMB"))
	r.Equal("cpu", hclName("CPU"))
	r.Equal("disk2_gb", hclName("Disk2GB"))
}

func TestHCLDiff_Values(t *testing.T) {
	r := require.New(t)

	// A number and a boolean read as they are, anything else as a string,
	// with what would end it early escaped.
	r.Equal("3", hclValue("3"))
	r.Equal("0.5", hclValue("0.5"))
	r.Equal("true", hclValue("true"))
	r.Equal(`"nginx:1.27"`, hclValue("nginx:1.27"))
	r.Equal(`"say \"hi\""`, hclValue(`say "hi"`))
	r.Equal(`"v1.2.3"`, hclValue("v1.2.3"))
}

func TestHCLDiff_AsTheJobFileWritesIt(t *testing.T) {
	r := require.New(t)

	diff := &api.JobDiff{Type: diffEdited, TaskGroups: []*api.TaskGroupDiff{{
		Type: diffEdited, Name: "g",
		Tasks: []*api.TaskDiff{{
			Type: diffEdited, Name: "t",
			Fields: []*api.FieldDiff{{Type: diffEdited, Name: "Env[V]", Old: "5", New: "6"}},
			Objects: []*api.ObjectDiff{
				{Type: diffEdited, Name: "Config", Fields: []*api.FieldDiff{
					{Type: diffEdited, Name: "args[1]", Old: "sleep 5", New: "sleep 7"},
				}},
				{Type: diffEdited, Name: "Resources", Fields: []*api.FieldDiff{
					{Type: diffEdited, Name: "MemoryMB", Old: "16", New: "24"},
					{Type: diffEdited, Name: "MemoryMaxMB", Old: "0", New: "64"},
				}},
			},
		}},
	}}}

	lines := hclDiff(diff)

	// An env value is a string, whatever it holds.
	r.Contains(lines, DiffLine{Kind: DiffAdded, Indent: 3, Text: `V = "6"`})

	// An element of a list is not a block of its own.
	r.Contains(lines, DiffLine{Kind: DiffAdded, Indent: 3, Text: `args[1] = "sleep 7"`})
	r.NotContains(lines, DiffLine{Kind: DiffContext, Indent: 3, Text: "args {"})

	// Memory is named as a job file names it.
	r.Contains(lines, DiffLine{Kind: DiffAdded, Indent: 3, Text: "memory = 24"})
	r.Contains(lines, DiffLine{Kind: DiffAdded, Indent: 3, Text: "memory_max = 64"})
}

func TestHCLDiff_NothingChanged(t *testing.T) {
	r := require.New(t)

	r.Empty(hclDiff(&api.JobDiff{Type: "None"}))
	r.Empty(hclDiff(nil))
}
