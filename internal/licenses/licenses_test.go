package licenses

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/licenseclassifier/v2/assets"
	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

var update = flag.Bool("update", false, "rewrite testdata/notices.golden")

// testdata returns the text of a file in testdata.
func testdata(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return string(b)
}

// writeFiles writes files, named by slash-separated paths, below dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()

	for name, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(text), 0o644))
	}
}

// publish writes a module version, with its files, into a GOPROXY directory.
func publish(t *testing.T, proxy string, mod module.Version, files map[string]string) {
	t.Helper()

	src := t.TempDir()
	writeFiles(t, src, files)

	versions := filepath.Join(proxy, filepath.FromSlash(mod.Path), "@v")
	writeFiles(t, versions, map[string]string{
		mod.Version + ".info": `{"Version":"` + mod.Version + `"}`,
		mod.Version + ".mod":  files["go.mod"],
	})

	f, err := os.Create(filepath.Join(versions, mod.Version+".zip"))
	require.NoError(t, err)
	require.NoError(t, zip.CreateFromDir(f, mod, src))
	require.NoError(t, f.Close())
}

// useProxy publishes module versions in a GOPROXY directory and points the go
// command at it, with an empty module cache, so the test sees what real
// dependencies look like.
func useProxy(t *testing.T, mods map[module.Version]map[string]string) {
	t.Helper()

	proxy := t.TempDir()
	for mod, files := range mods {
		publish(t, proxy, mod, files)
	}

	url := filepath.ToSlash(proxy)
	if !strings.HasPrefix(url, "/") {
		url = "/" + url // a Windows drive
	}

	t.Setenv("GOPROXY", "file://"+url)
	t.Setenv("GOMODCACHE", t.TempDir())
	t.Setenv("GOSUMDB", "off")
	t.Setenv("GOWORK", "off")
	// The test module gets its go.sum, and the module cache stays removable.
	t.Setenv("GOFLAGS", "-mod=mod -modcacherw")
}

// useApp writes a program that requires the modules and imports the
// packages, makes its directory the working directory and returns it.
func useApp(t *testing.T, mods []module.Version, imports ...string) string {
	t.Helper()

	var requires, imported strings.Builder
	for _, m := range mods {
		fmt.Fprintf(&requires, "require %s %s\n", m.Path, m.Version)
	}

	for _, p := range imports {
		fmt.Fprintf(&imported, "import _ %q\n", p)
	}

	app := t.TempDir()
	writeFiles(t, app, map[string]string{
		"go.mod":  "module example.com/app\n\ngo 1.26\n\n" + requires.String(),
		"LICENSE": "The program's own license is not a third-party notice.\n",
		"main.go": "package main\n\n" + imported.String() + "\nfunc main() {}\n",
	})
	t.Chdir(app)

	return app
}

var (
	modA   = module.Version{Path: "example.com/a", Version: "v1.0.0"}
	modLib = module.Version{Path: "example.com/lib", Version: "v1.0.0"}
	bsd    = []string{"BSD-3-Clause"}
)

// useLibraries publishes a, which links lib from a subpackage, and lib, which
// links win only on Windows and cgo only with cgo, and writes a program that
// links both.
func useLibraries(t *testing.T) {
	t.Helper()

	mit := testdata(t, "mit.txt")
	useProxy(t, map[module.Version]map[string]string{
		modA: {
			"go.mod": "module example.com/a\n\ngo 1.26\n\nrequire example.com/lib v1.0.0\n",
			// The only license is at the root, above the only linked package.
			"LICENSE": goLicense,
			"a.go":    "package a\n",
			// go list lists lib before a.
			"sub/sub.go": "package sub\n\nimport _ \"example.com/lib\"\n",
		},
		modLib: {
			"go.mod":         "module example.com/lib\n\ngo 1.26\n",
			"LICENSE":        mit,
			"NOTICE":         testdata(t, "apache-2.0-header.txt"),
			"README.md":      "Not a license.\n",
			"license.go":     "package lib\n",
			"lib.go":         "package lib\n",
			"lib_windows.go": "package lib\n\nimport _ \"example.com/lib/win\"\n",
			"win/win.go":     "package win\n",
			"win/LICENSE":    goLicense,
			"cgo/cgo.go":     "package cgo\n",
			"cgo/LICENSE":    mit,
		},
	})

	app := useApp(t, []module.Version{modA, modLib}, "example.com/a/sub", "example.com/lib")
	writeFiles(t, app, map[string]string{
		"cgo.go": "//go:build cgo\n\npackage main\n\nimport _ \"example.com/lib/cgo\"\n",
	})
}

// linkedStd and linkedA are what Linked finds in the standard library and in a.
func linkedStd() Module {
	return Module{
		Path: "std", Version: runtime.Version(), Source: "https://go.dev/dl/" + runtime.Version() + ".src.tar.gz",
		Files: []File{{Path: "LICENSE", Text: goLicense, Licenses: bsd}}, Licenses: bsd,
	}
}

func linkedA() Module {
	return Module{
		Path: modA.Path, Version: modA.Version, Source: "https://proxy.golang.org/example.com/a/@v/v1.0.0.zip",
		Files: []File{{Path: "LICENSE", Text: goLicense, Licenses: bsd}}, Licenses: bsd,
	}
}

// linkedLib is what Linked finds in lib on Linux, plus the extra files. It
// reads testdata, so it runs before useLibraries leaves the directory.
func linkedLib(t *testing.T, licenses []string, extra ...File) Module {
	t.Helper()

	// The notice holds only an Apache-2.0 header, which counts.
	files := []File{
		{Path: "LICENSE", Text: testdata(t, "mit.txt"), Licenses: []string{"MIT"}},
		{Path: "NOTICE", Text: testdata(t, "apache-2.0-header.txt"), Licenses: []string{"Apache-2.0"}},
	}

	return Module{
		Path: modLib.Path, Version: modLib.Version, Source: "https://proxy.golang.org/example.com/lib/@v/v1.0.0.zip",
		Files: append(files, extra...), Licenses: licenses,
	}
}

func TestLinked_FindsTheLicensesOfWhatThePlatformLinks(t *testing.T) {
	r := require.New(t)

	want := []Module{linkedStd(), linkedA(), linkedLib(t, []string{"Apache-2.0", "MIT"})}
	useLibraries(t)

	got, err := Linked(t.Context(), []string{"linux/amd64"}, ".")
	r.NoError(err)
	r.Equal(want, got)
}

func TestLinked_AddsWhatOnlyAnotherPlatformLinks(t *testing.T) {
	r := require.New(t)

	win := File{Path: "win/LICENSE", Text: goLicense, Licenses: bsd}
	want := []Module{linkedStd(), linkedA(), linkedLib(t, []string{"Apache-2.0", "BSD-3-Clause", "MIT"}, win)}
	useLibraries(t)

	got, err := Linked(t.Context(), []string{"linux/amd64", "windows/arm64"}, ".")
	r.NoError(err)
	r.Equal(want, got)
}

func TestLinked_LeavesCgoOutOnTheHost(t *testing.T) {
	r := require.New(t)

	useLibraries(t)

	// A cross build has no cgo anyway; a native one has it unless it is
	// turned off, and the release turns it off.
	host := runtime.GOOS + "/" + runtime.GOARCH
	got, err := Linked(t.Context(), []string{host}, ".")
	r.NoError(err)

	i := slices.IndexFunc(got, func(m Module) bool { return m.Path == modLib.Path })
	r.GreaterOrEqual(i, 0, "Linked(%s) = %+v, without %s", host, got, modLib.Path)
	r.False(slices.ContainsFunc(got[i].Files, func(f File) bool { return f.Path == "cgo/LICENSE" }),
		"Linked(%s) builds with cgo", host)
}

func TestCheck_FailsOnAnUnknownLicenseBesideAKnownOne(t *testing.T) {
	r := require.New(t)

	// A known license in a subpackage must not hide a proprietary license at
	// the root.
	prop := module.Version{Path: "example.com/prop", Version: "v1.0.0"}
	useProxy(t, map[module.Version]map[string]string{prop: {
		"go.mod":      "module example.com/prop\n\ngo 1.26\n",
		"LICENSE":     "Copyright 2026 Example Corp. All rights reserved. Use needs a written agreement.\n",
		"sub/LICENSE": testdata(t, "mit.txt"),
		"sub/sub.go":  "package sub\n",
	}})
	useApp(t, []module.Version{prop}, "example.com/prop/sub")

	mods, err := Linked(t.Context(), []string{"linux/amd64"}, ".")
	r.NoError(err)
	r.EqualError(Check(mods, Allowed), "example.com/prop v1.0.0: LICENSE: no license found")
}

func TestGoList_NamesThePlatformWhenItFails(t *testing.T) {
	_, err := goList(t.Context(), "windows/arm64", []string{"./does-not-exist"})
	require.ErrorContains(t, err, "list packages for windows/arm64: ")
}

func TestCollect_RefusesModulesItCannotName(t *testing.T) {
	downloaded := &listedModule{Path: "example.com/x", Version: "v1.0.0", Dir: "/x"}
	notDownloaded, replaced := *downloaded, *downloaded
	notDownloaded.Dir = ""
	replaced.Replace = &listedModule{Path: "../fork", Dir: "/fork"}

	cases := []struct {
		name   string
		module *listedModule
		says   string
	}{
		{"no module", nil, "package example.com/x is in no module"},
		{
			"not downloaded", &notDownloaded,
			"module example.com/x v1.0.0 is not in the module cache: run go mod download",
		},
		// The notices would name the source of the module that was replaced.
		{
			"replaced", &replaced,
			"module example.com/x v1.0.0 is replaced by ../fork, and the notices cannot name its source",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := collect([]listedPackage{{ImportPath: "example.com/x", Dir: "/x", Module: c.module}})
			require.EqualError(t, err, c.says)
		})
	}
}

func TestCheck_PassesAllowedLicensesAndBareNotices(t *testing.T) {
	ok := Module{Path: "example.com/ok", Version: "v1.0.0", Licenses: []string{"Apache-2.0", "MIT"}, Files: []File{
		{Path: "LICENSE", Licenses: []string{"MIT"}},
		{Path: "NOTICE", Licenses: []string{"Apache-2.0"}},
		// A notice need not name a license.
		{Path: "sub/NOTICE.txt"},
	}}

	require.NoError(t, Check([]Module{ok}, []string{"Apache-2.0", "MIT"}))
}

func TestCheck_NamesEveryUnknownOrDisallowedLicense(t *testing.T) {
	mods := []Module{
		{
			Path: "example.com/gpl", Version: "v1.2.0", Licenses: []string{"GPL-3.0", "MIT"},
			Files: []File{{Path: "COPYING", Licenses: []string{"GPL-3.0", "MIT"}}},
		},
		// A license the classifier does not know, such as BUSL-1.1, fails
		// beside a known one too.
		{
			Path: "example.com/busl", Version: "v2.0.0", Licenses: []string{"MIT"},
			Files: []File{{Path: "LICENSE"}, {Path: "sub/LICENSE", Licenses: []string{"MIT"}}},
		},
		{Path: "example.com/notice", Version: "v0.2.0", Files: []File{{Path: "NOTICE"}}},
		{Path: "example.com/none", Version: "v0.1.0"},
	}

	require.EqualError(t, Check(mods, []string{"Apache-2.0", "MIT"}), strings.Join([]string{
		"example.com/gpl v1.2.0: GPL-3.0 is not an allowed license",
		"example.com/busl v2.0.0: LICENSE: no license found",
		"example.com/notice v0.2.0: no license found",
		"example.com/none v0.1.0: no license found",
	}, "\n"))
}

func TestAllowed_IsKnownToTheClassifier(t *testing.T) {
	// A name the classifier does not use never matches, so the list would
	// allow less than it says.
	for _, name := range Allowed {
		_, err := assets.ReadLicenseFile("License/" + name)
		require.NotErrorIs(t, err, fs.ErrNotExist, "the classifier does not know %s", name)
	}
}

func TestNotices_MatchTheGoldenFile(t *testing.T) {
	const golden = "notices.golden"

	mods := []Module{
		{
			Path: "std", Version: "go1.26.0", Source: "https://go.dev/dl/go1.26.0.src.tar.gz",
			Files:    []File{{Path: "LICENSE", Text: "Copyright 2009 The Go Authors.\n"}},
			Licenses: []string{"BSD-3-Clause"},
		},
		{
			Path: "example.com/lib", Version: "v1.0.0",
			Source: "https://proxy.golang.org/example.com/lib/@v/v1.0.0.zip",
			Files: []File{
				{Path: "LICENSE", Text: "The MIT license.\n"},
				{Path: "sub/NOTICE", Text: "A notice without a final newline."},
			},
			Licenses: []string{"Apache-2.0", "MIT"},
		},
	}

	got := Notices(mods)
	if *update {
		require.NoError(t, os.WriteFile(filepath.Join("testdata", golden), got, 0o644))
	}

	require.Equal(t, testdata(t, golden), string(got))
}

func TestGoLicense_IsTheOneGoShips(t *testing.T) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	require.NoError(t, err)

	path := filepath.Join(strings.TrimSpace(string(out)), "LICENSE")

	shipped, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// Homebrew and Linux distributions move it; CI installs Go from
		// go.dev, which keeps it.
		if os.Getenv("CI") != "" {
			t.Fatalf("%s does not exist", path)
		}

		t.Skipf("%s does not exist: this Go distribution moved it", path)
	}

	require.NoError(t, err)
	require.Equal(t, string(shipped), goLicense, "go.LICENSE differs from %s; copy it", path)
}
