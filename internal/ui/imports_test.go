package ui

import (
	"strconv"
	"testing"
)

// Only internal/nomad talks to the cluster's API: a type of the API used
// here ties a screen to how the cluster spells a value.
func TestUI_DoesNotImportTheNomadAPI(t *testing.T) {
	fset, files := sourceFiles(t)

	for _, file := range files {
		for _, imp := range file.Imports {
			if path, _ := strconv.Unquote(imp.Path.Value); path == "github.com/hashicorp/nomad/api" {
				t.Errorf("%s imports %s", fset.Position(imp.Pos()), path)
			}
		}
	}
}
