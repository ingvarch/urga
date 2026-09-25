package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ingvarch/urga/internal/nomad"
)

var deployBot = nomad.Token{Name: "deploy-bot", Type: "client"}

// jobsSeenWith is the list of jobs as a session with that token was
// answered.
func jobsSeenWith(token nomad.Token, jobs []nomad.Job) Model {
	m := New(&fakeClient{}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(tokenMsg(token))
	m, _ = m.update(jobsMsg(jobs))

	return m
}

func TestEmptyList_AClientTokenMayNotReachIt(t *testing.T) {
	r := require.New(t)

	// A token that reads one namespace is answered an empty list for all of
	// them, and no error.
	m := jobsSeenWith(deployBot, nil)

	r.Contains(plain(m.render()), "Nothing here that deploy-bot can read: its policies may not allow it.")
	r.Contains(m.render(), opening(styleMuted)+"Nothing here")
}

func TestEmptyList_NoTokenSet(t *testing.T) {
	r := require.New(t)

	m := jobsSeenWith(nomad.Token{Anonymous: true}, nil)

	r.Contains(plain(m.render()), "Nothing here: no token is set, and anonymous access may not allow it.")
}

func TestEmptyList_NothingToSayAboutTheToken(t *testing.T) {
	r := require.New(t)

	// A management token reads everything, a cluster without ACLs checks
	// nothing: the list is empty because it is.
	for _, token := range []nomad.Token{{Name: "ops", Type: "management"}, {ACLsOff: true}} {
		r.NotContains(plain(jobsSeenWith(token, nil).render()), "Nothing here")
	}

	// Before the cluster said whose token it is.
	m := New(&fakeClient{}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(jobsMsg(nil))
	r.NotContains(plain(m.render()), "Nothing here")
}

func TestEmptyList_OnlyOnceItIsAnswered(t *testing.T) {
	r := require.New(t)

	m := New(&fakeClient{}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(tokenMsg(deployBot))

	// The list is on its way: empty is not an answer yet.
	r.NotContains(plain(m.render()), "Nothing here")

	m, _ = m.update(jobsMsg(nil))
	r.Contains(plain(m.render()), "Nothing here")

	// Another screen opens: its list is on its way again.
	m, _ = m.show(deploymentsView)
	r.NotContains(plain(m.render()), "Nothing here")
}

func TestEmptyList_AFilterIsNotTheToken(t *testing.T) {
	r := require.New(t)

	m := jobsSeenWith(deployBot, twoJobs())
	m.list.filter = "nothing-matches-this"
	m.layout()

	r.NotContains(plain(m.render()), "Nothing here")
}

// forbidden is what the cluster answers a request the token may not make.
func forbidden(t *testing.T) error {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Permission denied", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	client, err := nomad.New(nomad.Config{Address: server.URL})
	require.NoError(t, err)

	_, err = client.Jobs(context.Background(), "default")
	require.Error(t, err)

	return err
}

func TestPermissionDenied_SaysWhy(t *testing.T) {
	err := forbidden(t)

	for name, tc := range map[string]struct {
		token nomad.Token
		says  string
	}{
		"a client token": {deployBot, "Permission denied: deploy-bot may not do this"},
		"no token":       {nomad.Token{Anonymous: true}, "Permission denied: no token is set"},
		"a refused one":  {nomad.Token{Refused: true}, "Permission denied: the token is not valid"},
	} {
		t.Run(name, func(t *testing.T) {
			m := jobsSeenWith(tc.token, twoJobs())
			m, _ = m.update(errMsg{err: err})

			require.Contains(t, plain(statusLine(m)), tc.says)
		})
	}

	// What a management token is refused, the cluster says best itself.
	m := jobsSeenWith(nomad.Token{Name: "ops", Type: "management"}, twoJobs())
	m, _ = m.update(errMsg{err: err})
	require.Contains(t, plain(statusLine(m)), "403")
}

func TestPermissionDenied_BeforeTheTokenIsKnown(t *testing.T) {
	r := require.New(t)

	// The list and the token are asked at once; the list can be refused
	// before the cluster said whose token it is.
	m := New(&fakeClient{}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(errMsg{err: forbidden(t)})
	r.Contains(plain(statusLine(m)), "403")

	m, _ = m.update(tokenMsg(nomad.Token{Anonymous: true}))
	r.Contains(plain(statusLine(m)), "Permission denied: no token is set")
}

func TestPermissionDenied_AMessageSaidSinceStays(t *testing.T) {
	r := require.New(t)

	m := New(&fakeClient{}, Options{Version: "v-test"})
	m, _ = m.update(sizeMsg())
	m, _ = m.update(errMsg{err: forbidden(t)})

	// Something else was said before the token was known.
	m = m.say("Job web submitted.")
	m, _ = m.update(tokenMsg(nomad.Token{Anonymous: true}))

	r.Contains(plain(statusLine(m)), "Job web submitted.")
}
