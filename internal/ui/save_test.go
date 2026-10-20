package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// savedAt is the path the status line says the file went to.
func savedAt(t *testing.T, m Model) string {
	t.Helper()

	text := m.flash.text
	require.True(t, strings.HasPrefix(text, "Saved to "), text)
	require.True(t, strings.HasSuffix(text, "."), text)

	return strings.TrimSuffix(strings.TrimPrefix(text, "Saved to "), ".")
}

// sameFile checks that two paths name the same place: the temp folder of the
// system can be reached through a link.
func sameFile(t *testing.T, want, got string) {
	t.Helper()

	dir, err := filepath.EvalSymlinks(filepath.Dir(want))
	require.NoError(t, err)

	other, err := filepath.EvalSymlinks(filepath.Dir(got))
	require.NoError(t, err)

	require.Equal(t, filepath.Join(dir, filepath.Base(want)), filepath.Join(other, filepath.Base(got)))
}

// saveTyped presses ctrl+s, replaces the name with path, and presses enter.
func saveTyped(t *testing.T, m Model, path string) Model {
	t.Helper()

	m, _ = m.update(ctrlKey('s'))
	m, _ = m.update(ctrlKey('u'))
	m = typeIn(m, path)
	m, cmd := m.update(enter())

	return drain(m, cmd)
}

func TestSave_OpensTheLineWithTheNameAndWritesNothingYet(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := onLogs(t, "ready to serve\n")

	m, cmd := m.update(ctrlKey('s'))
	r.Nil(cmd)
	r.Equal(overlayAnswer, m.overlay)
	r.Contains(plain(m.render()), "save to: server-stdout-")
	r.Contains(plain(m.render()), ".log")

	files, err := os.ReadDir(dir)
	r.NoError(err)
	r.Empty(files)
}

func TestSave_EnterKeepsTheName(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := onLogs(t, "ready to serve\n", "connection refused\n")

	m, _ = m.update(ctrlKey('s'))
	m, cmd := m.update(enter())
	m = drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "server-stdout-*.log"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Equal("ready to serve\nconnection refused", strings.TrimSpace(string(saved)))

	sameFile(t, files[0], savedAt(t, m))
	r.True(filepath.IsAbs(savedAt(t, m)))
}

func TestSave_TheHomeFolderIsTheTilde(t *testing.T) {
	r := require.New(t)

	t.Chdir(t.TempDir())

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	m, _ := onLogs(t, "ready to serve\n")
	m = saveTyped(t, m, "~/out/web.log")

	saved, err := os.ReadFile(filepath.Join(home, "out", "web.log"))
	r.NoError(err)
	r.Contains(string(saved), "ready to serve")
	sameFile(t, filepath.Join(home, "out", "web.log"), savedAt(t, m))
}

func TestSave_ARelativePathIsFromWhereUrgaRuns(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := onLogs(t, "ready to serve\n")
	m = saveTyped(t, m, "logs/a/web.log")

	saved, err := os.ReadFile(filepath.Join(dir, "logs", "a", "web.log"))
	r.NoError(err)
	r.Contains(string(saved), "ready to serve")
	sameFile(t, filepath.Join(dir, "logs", "a", "web.log"), savedAt(t, m))
}

func TestSave_AnExistingFileIsAskedAboutFirst(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	target := filepath.Join(dir, "web.log")
	r.NoError(os.WriteFile(target, []byte("old"), 0o600))

	m, _ := onLogs(t, "ready to serve\n")
	m = saveTyped(t, m, "web.log")

	r.Contains(plain(m.render()), "Really overwrite ")

	// Read from the question: the window cuts a long path.
	question := m.confirm.question
	r.True(strings.HasPrefix(question, "Really overwrite "), question)
	r.True(strings.HasSuffix(question, "?"), question)

	asked := strings.TrimSuffix(strings.TrimPrefix(question, "Really overwrite "), "?")
	r.True(filepath.IsAbs(asked), asked)
	sameFile(t, target, asked)

	m, _ = m.update(key('n'))

	kept, err := os.ReadFile(target)
	r.NoError(err)
	r.Equal("old", string(kept))

	m = saveTyped(t, m, "web.log")

	m, cmd := m.update(key('y'))
	m = drain(m, cmd)

	saved, err := os.ReadFile(target)
	r.NoError(err)
	r.Contains(string(saved), "ready to serve")
	sameFile(t, target, savedAt(t, m))
}

func TestSave_AFolderIsNotWrittenOver(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)
	r.NoError(os.Mkdir(filepath.Join(dir, "out"), 0o755))

	m, _ := onLogs(t, "ready to serve\n")
	m = saveTyped(t, m, "./out")

	r.Contains(m.flash.text, "cannot save to ")
	r.Contains(m.flash.text, string(filepath.Separator)+"out: it is a folder")
	r.NotContains(m.flash.text, "./out")
	r.NotContains(plain(m.render()), "Really overwrite")

	entries, err := os.ReadDir(filepath.Join(dir, "out"))
	r.NoError(err)
	r.Empty(entries)
}

func TestSave_AFolderThatIsABrokenLinkFailsWithoutAQuestion(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)
	r.NoError(os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "link")))

	m, _ := onLogs(t, "ready to serve\n")
	m = saveTyped(t, m, "link/x.log")

	r.True(strings.HasPrefix(m.flash.text, "cannot save to link/x.log: "), m.flash.text)
	r.NotContains(m.flash.text, "mkdir", "the reason does not repeat the path")
	r.NotEqual(overlayConfirm, m.overlay)
	r.NotContains(plain(m.render()), "Really overwrite")
}

func TestSave_EscSavesNothingAndAnEmptyLineWarns(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := onLogs(t, "ready to serve\n")

	m, _ = m.update(ctrlKey('s'))
	m, cmd := m.update(escape())
	m = drain(m, cmd)

	r.Equal(overlayNone, m.overlay)

	files, err := os.ReadDir(dir)
	r.NoError(err)
	r.Empty(files)

	m = saveTyped(t, m, "")
	r.Equal("Saving needs a path.", m.flash.text)

	files, err = os.ReadDir(dir)
	r.NoError(err)
	r.Empty(files)
}

func TestSave_ALineThatArrivesWhileTheLineIsOpenIsInTheFile(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m, _ := onLogs(t, "ready to serve\n")

	m, _ = m.update(ctrlKey('s'))
	m = withLines(m, []string{"connection refused\n"})

	m, cmd := m.update(enter())
	drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Contains(string(saved), "connection refused")
}

func TestSave_ADescriptionSavesTheSameWay(t *testing.T) {
	r := require.New(t)

	dir := t.TempDir()
	t.Chdir(dir)

	m := described(t, `{"ID": "web"}`)

	m, _ = m.update(ctrlKey('s'))
	r.Contains(plain(m.render()), "save to: ")

	m, cmd := m.update(enter())
	m = drain(m, cmd)

	files, err := filepath.Glob(filepath.Join(dir, "*.txt"))
	r.NoError(err)
	r.Len(files, 1)

	saved, err := os.ReadFile(files[0])
	r.NoError(err)
	r.Contains(string(saved), `"ID": "web"`)
	sameFile(t, files[0], savedAt(t, m))
}
