package ui

import (
	"go/ast"
	"go/token"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

// These lists are kept in step by hand: an entry left out of one compiles,
// and fails only in front of the user.

// A view left out of views cannot be opened from the command line, and is
// not restored from the saved session.
func TestViews_ListEveryView(t *testing.T) {
	r := require.New(t)

	_, files := sourceFiles(t)
	makers := viewMakers(files)

	listed := map[string]bool{}
	declared := []string{}

	for _, file := range files {
		for name, value := range packageVars(file) {
			if name == "views" {
				for _, elt := range value.(*ast.CompositeLit).Elts {
					listed[elt.(*ast.Ident).Name] = true
				}

				continue
			}

			if isView(value, makers) {
				declared = append(declared, name)
			}
		}
	}

	r.NotEmpty(declared)

	for _, name := range declared {
		r.True(listed[name], "%s is not in views", name)
	}
}

// packageVars are the variables a file declares at the top, by name, with
// the value each starts with.
func packageVars(file *ast.File) map[string]ast.Expr {
	out := map[string]ast.Expr{}

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			for i, name := range value.Names {
				if i < len(value.Values) {
					out[name.Name] = value.Values[i]
				}
			}
		}
	}

	return out
}

// viewMakers are the functions of the package that return a *view.
func viewMakers(files []*ast.File) map[string]bool {
	out := map[string]bool{}

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
				continue
			}

			if isViewPointer(fn.Type.Results.List[0].Type) {
				out[fn.Name.Name] = true
			}
		}
	}

	return out
}

func isViewPointer(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}

	id, ok := star.X.(*ast.Ident)

	return ok && id.Name == "view"
}

// isView says a value is &view{...}, or what a function that makes views
// returns.
func isView(value ast.Expr, makers map[string]bool) bool {
	switch v := value.(type) {
	case *ast.UnaryExpr:
		lit, ok := v.X.(*ast.CompositeLit)
		if !ok {
			return false
		}

		id, ok := lit.Type.(*ast.Ident)

		return ok && id.Name == "view"
	case *ast.CallExpr:
		id, ok := v.Fun.(*ast.Ident)

		return ok && makers[id.Name]
	}

	return false
}

// enter is offered on a match exactly when it opens something: a kind that
// opens nothing would offer a key that does nothing.
func TestFind_EnterIsOfferedForWhatOpens(t *testing.T) {
	for _, kind := range nomad.MatchKinds {
		t.Run(kind, func(t *testing.T) {
			match := nomad.Match{Kind: kind, Name: "web", ID: "web", Namespace: "production", JobID: "web", Group: "frontend", Task: "server"}
			p := findPage{found: nomad.Found{Matches: []nomad.Match{match}}}
			e := env{client: &fakeClient{}, onRow: true}

			opened := openMatch(e.client, match)
			opens := opened.cmd != nil || len(opened.now) > 0

			offered := slices.ContainsFunc(p.keys(e), func(k keyHint) bool { return k.press == "enter" && k.offered })

			require.Equal(t, opens, offered)
		})
	}
}

// A word the prompt offers for a scope is one it takes, and every scope it
// takes is offered by a word.
func TestScopes_OfferedAreTaken(t *testing.T) {
	r := require.New(t)

	offered := map[scope]bool{}

	for _, name := range scopeNames {
		s, ok := scopeAliases[name]
		r.True(ok, "%s is offered and not taken", name)

		offered[s] = true
	}

	for name, s := range scopeAliases {
		r.True(offered[s], "the scope of %s is taken and never offered", name)
	}
}
