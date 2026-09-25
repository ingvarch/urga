package main

import (
	"flag"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
	"github.com/ingvarch/urga/internal/settings"
)

// flagsFrom parses a command line with a namespace flag whose default came
// from the environment.
func flagsFrom(t *testing.T, args ...string) *flag.FlagSet {
	t.Helper()

	flags := flag.NewFlagSet("urga", flag.ContinueOnError)
	flags.String("namespace", "from-env", "")
	require.NoError(t, flags.Parse(args))

	return flags
}

func TestGiven(t *testing.T) {
	r := require.New(t)

	// A default is not a choice: it comes from the environment of every run.
	r.False(given(flagsFrom(t), "namespace"))

	r.True(given(flagsFrom(t, "-namespace", "production"), "namespace"))

	// Naming the default is a choice all the same.
	r.True(given(flagsFrom(t, "-namespace", "from-env"), "namespace"))
}

func TestParseFlags(t *testing.T) {
	r := require.New(t)

	cl, err := parseFlags([]string{"-version", "-cluster", "prod", "-address", "http://10.0.0.9:4646",
		"-region", "us", "-namespace", "batch", "-readonly"})
	r.NoError(err)

	r.True(cl.version)
	r.Equal("prod", cl.cluster)
	r.Equal("http://10.0.0.9:4646", cl.address)
	r.Equal("us", cl.region)
	r.Equal("batch", cl.namespace)
	r.True(cl.namespaceGiven)
	r.True(cl.readOnly)
}

func TestParseFlags_Help(t *testing.T) {
	// run exits quietly on help, so the error has to come back as it is.
	_, err := parseFlags([]string{"-h"})
	require.ErrorIs(t, err, flag.ErrHelp)
}

// twoClusters are the settings of a user with a dev and a prod cluster.
func twoClusters() settings.Settings {
	return settings.Settings{
		Default: "dev",
		Clusters: map[string]settings.Cluster{
			"dev":  {Address: "http://127.0.0.1:4646", Namespace: "default"},
			"prod": {Address: "https://nomad.prod:4646", Region: "eu", Token: "the-token-of-prod", ReadOnly: true, CACert: "/ca.pem", Color: "red"},
		},
	}
}

func startFrom(t *testing.T, s settings.Settings, args ...string) (start, error) {
	t.Helper()

	cl, err := parseFlags(args)
	require.NoError(t, err)

	return startOn(cl, s)
}

func TestStartOn_TheEnvironment(t *testing.T) {
	r := require.New(t)

	t.Setenv("NOMAD_NAMESPACE", "from-env")

	// Without a settings file, urga runs as it always did.
	st, err := startFrom(t, settings.Settings{}, "-address", "http://10.0.0.1:4646")
	r.NoError(err)

	r.Empty(st.cluster)
	r.Equal(nomad.Config{Address: "http://10.0.0.1:4646"}, st.nomad)
	r.Equal("from-env", st.namespace)
	r.False(st.namespaceGiven)
}

func TestStartOn_TheDefaultCluster(t *testing.T) {
	r := require.New(t)

	// The environment is for the cluster urga runs without settings.
	t.Setenv("NOMAD_NAMESPACE", "from-env")

	st, err := startFrom(t, twoClusters())
	r.NoError(err)

	r.Equal("dev", st.cluster)
	r.Equal(nomad.Config{Named: true, Address: "http://127.0.0.1:4646"}, st.nomad)
	r.Equal("default", st.namespace)
	r.False(st.readOnly)
}

func TestStartOn_AClusterByName(t *testing.T) {
	r := require.New(t)

	st, err := startFrom(t, twoClusters(), "-cluster", "prod")
	r.NoError(err)

	r.Equal("prod", st.cluster)
	r.Equal(nomad.Config{
		Named:   true,
		Address: "https://nomad.prod:4646",
		Region:  "eu",
		Token:   "the-token-of-prod",
		TLS:     nomad.TLS{CACert: "/ca.pem"},
	}, st.nomad)

	// Read-only is the settings or the command line, whichever says so.
	r.True(st.readOnly)
	r.Empty(st.namespace)
	r.Equal("red", st.color)
}

func TestStartOn_TheCommandLineWins(t *testing.T) {
	r := require.New(t)

	st, err := startFrom(t, twoClusters(), "-cluster", "dev", "-address", "http://10.0.0.9:4646",
		"-region", "us", "-namespace", "batch", "-readonly")
	r.NoError(err)

	r.Equal("http://10.0.0.9:4646", st.nomad.Address)
	r.Equal("us", st.nomad.Region)
	r.True(st.nomad.Named)
	r.Equal("batch", st.namespace)
	r.True(st.namespaceGiven)
	r.True(st.readOnly)
}

func TestStartOn_AClusterThatIsNotThere(t *testing.T) {
	r := require.New(t)

	_, err := startFrom(t, twoClusters(), "-cluster", "stage")
	r.ErrorContains(err, `no cluster "stage": dev, prod`)

	_, err = startFrom(t, settings.Settings{}, "-cluster", "prod")
	r.ErrorContains(err, `no cluster "prod"`)
}

func TestStartOn_ATokenThatCannotBeRead(t *testing.T) {
	s := twoClusters()
	s.Clusters["prod"] = settings.Cluster{Address: "https://nomad.prod:4646", TokenEnv: "NOMAD_TOKEN_NOWHERE"}

	_, err := startFrom(t, s, "-cluster", "prod")
	require.ErrorContains(t, err, "NOMAD_TOKEN_NOWHERE")
}

func TestConnect(t *testing.T) {
	r := require.New(t)

	cl, err := parseFlags([]string{"-cluster", "dev", "-address", "http://10.0.0.9:4646"})
	r.NoError(err)

	// A CA file that is not there would stop the connection.
	s := twoClusters()
	prod := s.Clusters["prod"]
	prod.CACert = ""
	s.Clusters["prod"] = prod

	conn, err := connectWith(cl, s)("prod")
	r.NoError(err)

	// prod as the settings say it is: the address of the command line was
	// for the cluster urga started on.
	r.Equal("prod", conn.Name)
	r.Equal("red", conn.Color)
	r.True(conn.ReadOnly)
	r.Equal("https://nomad.prod:4646", conn.Client.Address())
	r.NotNil(conn.InRegion)
	r.NotNil(conn.Shell)
}

func TestConnect_ReadOnlyForEveryCluster(t *testing.T) {
	r := require.New(t)

	cl, err := parseFlags([]string{"-readonly"})
	r.NoError(err)

	// Asked for on the command line, it holds wherever the session goes.
	conn, err := connectWith(cl, twoClusters())("dev")
	r.NoError(err)
	r.True(conn.ReadOnly)
	r.Equal("default", conn.Namespace)
}

func TestConnect_AClusterThatIsNotThere(t *testing.T) {
	cl, err := parseFlags(nil)
	require.NoError(t, err)

	_, err = connectWith(cl, twoClusters())("stage")
	require.ErrorContains(t, err, `no cluster "stage"`)
}

// environment is a set of variables as the process would read them.
func environment(vars map[string]string) func(string) string {
	return func(name string) string { return vars[name] }
}

func TestNewerRelease_OnlyARelease(t *testing.T) {
	r := require.New(t)

	none := environment(nil)

	r.NotNil(newerRelease(none, "v0.5.0"))

	// A build from source has no release to be behind.
	r.Nil(newerRelease(none, "dev"))
	r.Nil(newerRelease(none, "v0.5.0-3-gabc1234-dirty"))
}

func TestNewerRelease_TurnedOff(t *testing.T) {
	r := require.New(t)

	// Any value turns it off, the way NO_COLOR does.
	r.Nil(newerRelease(environment(map[string]string{"URGA_NO_UPDATE_CHECK": "1"}), "v0.5.0"))
	r.Nil(newerRelease(environment(map[string]string{"URGA_NO_UPDATE_CHECK": "yes"}), "v0.5.0"))
	r.NotNil(newerRelease(environment(map[string]string{"URGA_NO_UPDATE_CHECK": ""}), "v0.5.0"))
}
