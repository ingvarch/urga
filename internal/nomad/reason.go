package nomad

import (
	"errors"
	"strings"

	"github.com/hashicorp/nomad/api"
)

// Reason is what an error says, with what the cluster answered in place of
// the code the client wraps it in: "job not found", not "Unexpected
// response code: 500 (job not found)". What urga says around it stays.
func Reason(err error) string {
	said := err.Error()

	var answer api.UnexpectedResponseError
	if !errors.As(err, &answer) || !answer.HasBody() {
		return said
	}

	return strings.Replace(said, answer.Error(), errorsOf(answer.Body()), 1)
}

// errorsOf is what a body the cluster writes as a list of errors under a count
// says: "2 errors occurred:\n\t* first\n\t* second" is "first; second". Any
// other body is as it is.
func errorsOf(body string) string {
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) < 2 || !strings.HasSuffix(lines[0], "occurred:") {
		return body
	}

	errs := make([]string, 0, len(lines)-1)

	for _, line := range lines[1:] {
		if text, ok := strings.CutPrefix(strings.TrimSpace(line), "* "); ok {
			errs = append(errs, text)
		}
	}

	if len(errs) == 0 {
		return body
	}

	return strings.Join(errs, "; ")
}
