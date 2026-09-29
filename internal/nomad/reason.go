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

	return strings.Replace(said, answer.Error(), answer.Body(), 1)
}
