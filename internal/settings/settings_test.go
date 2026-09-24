package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/settings"
)

// write puts a settings file where Load reads it.
func write(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "clusters.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

const twoClusters = `
default = "dev"

[clusters.dev]
address   = "http://127.0.0.1:4646"
namespace = "default"

[clusters.prod]
address         = "https://nomad.prod:4646"
region          = "eu"
token_command   = ["op", "read", "op://ops/nomad/token"]
ca_cert         = "~/.nomad/prod-ca.pem"
client_cert     = "/etc/nomad/cli.pem"
client_key      = "/etc/nomad/cli-key.pem"
tls_server_name = "server.eu.nomad"
read_only       = true
color           = "red"
`

func TestLoad(t *testing.T) {
	r := require.New(t)

	home := t.TempDir()
	t.Setenv("HOME", home)

	s, err := settings.Load(write(t, twoClusters))
	r.NoError(err)

	r.Equal("dev", s.Default)
	r.Equal([]string{"dev", "prod"}, s.Names())

	r.Equal(settings.Cluster{Address: "http://127.0.0.1:4646", Namespace: "default"}, s.Clusters["dev"])

	// A path that starts at home is read from home.
	r.Equal(settings.Cluster{
		Address:       "https://nomad.prod:4646",
		Region:        "eu",
		TokenCommand:  []string{"op", "read", "op://ops/nomad/token"},
		CACert:        filepath.Join(home, ".nomad/prod-ca.pem"),
		ClientCert:    "/etc/nomad/cli.pem",
		ClientKey:     "/etc/nomad/cli-key.pem",
		TLSServerName: "server.eu.nomad",
		ReadOnly:      true,
		Color:         "red",
	}, s.Clusters["prod"])
}

func TestLoad_NoFile(t *testing.T) {
	r := require.New(t)

	// Without a file urga runs from the environment, as it always did.
	s, err := settings.Load(filepath.Join(t.TempDir(), "clusters.toml"))
	r.NoError(err)
	r.Empty(s.Names())
	r.Empty(s.Default)
}

func TestLoad_Refuses(t *testing.T) {
	for name, tc := range map[string]struct{ content, says string }{
		// A typo in a file that holds tokens must not pass unnoticed.
		"an unknown key": {`
[clusters.prod]
address = "https://nomad.prod:4646"
read_onyl = true
`, "clusters.prod.read_onyl"},
		"a default that is not a cluster": {`
default = "stage"

[clusters.prod]
address = "https://nomad.prod:4646"
`, `default "stage"`},
		"a cluster without an address": {`
[clusters.prod]
region = "eu"
`, `cluster "prod" has no address`},
		"two token sources": {`
[clusters.prod]
address   = "https://nomad.prod:4646"
token     = "secret"
token_env = "NOMAD_TOKEN_PROD"
`, `cluster "prod": one of token, token_env and token_command`},
		"a colour not in the palette": {`
[clusters.prod]
address = "https://nomad.prod:4646"
color   = "pink"
`, `"pink" is not a colour: red, orange, yellow, green, cyan, blue, purple`},
		"what is not TOML": {`[clusters.prod`, "clusters.toml"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := settings.Load(write(t, tc.content))
			require.ErrorContains(t, err, tc.says)
		})
	}
}

func TestToken(t *testing.T) {
	r := require.New(t)

	token, err := settings.Cluster{Token: "plain"}.ReadToken()
	r.NoError(err)
	r.Equal("plain", token)

	t.Setenv("NOMAD_TOKEN_PROD", "from-env")
	token, err = settings.Cluster{TokenEnv: "NOMAD_TOKEN_PROD"}.ReadToken()
	r.NoError(err)
	r.Equal("from-env", token)

	// What the command prints, without the line break it ends with.
	token, err = settings.Cluster{TokenCommand: []string{"sh", "-c", "echo from-command"}}.ReadToken()
	r.NoError(err)
	r.Equal("from-command", token)

	// No source is no token, not the one of the environment.
	t.Setenv("NOMAD_TOKEN", "the-environment")
	token, err = settings.Cluster{}.ReadToken()
	r.NoError(err)
	r.Empty(token)
}

func TestToken_ThatCannotBeRead(t *testing.T) {
	r := require.New(t)

	// A variable that is not set is a mistake to point out, not an empty
	// token to send.
	_, err := settings.Cluster{TokenEnv: "NOMAD_TOKEN_NOWHERE"}.ReadToken()
	r.ErrorContains(err, "NOMAD_TOKEN_NOWHERE is not set")

	// A command that fails says what it said.
	_, err = settings.Cluster{TokenCommand: []string{"sh", "-c", "echo locked >&2; exit 1"}}.ReadToken()
	r.ErrorContains(err, "locked")
}

func TestColours(t *testing.T) {
	require.Equal(t, []string{"red", "orange", "yellow", "green", "cyan", "blue", "purple"}, settings.Colours)
}
