package nomad

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/nomad/api"
)

// Token is the ACL token the session sends, as the cluster sees it.
type Token struct {
	// ID is the accessor of the token; it names a token without a name.
	ID string

	Name string

	// Type is client or management: a management token reads everything.
	Type string

	// Expires is when the token stops working, zero for never.
	Expires time.Time

	// Anonymous says no token was sent; ACLsOff that the cluster has no
	// ACLs to check one against; Refused that the cluster does not know
	// the token that was sent, or no longer.
	Anonymous bool
	ACLsOff   bool
	Refused   bool
}

// The accessor IDs the cluster returns in place of a real token: none was
// sent, or ACLs are off.
const (
	anonymousAccessor = "anonymous"
	aclsOffAccessor   = "acls-disabled"
)

// Token asks the cluster about the token the session sends. A token it
// refuses sets Refused and returns no error.
func (c *Client) Token(ctx context.Context) (Token, error) {
	self, _, err := c.api.ACLTokens().Self(c.query(ctx, ""))
	if Forbidden(err) {
		return Token{Refused: true}, nil
	}

	if err != nil {
		return Token{}, err
	}

	token := Token{
		ID:        self.AccessorID,
		Name:      self.Name,
		Type:      self.Type,
		Anonymous: self.AccessorID == anonymousAccessor,
		ACLsOff:   self.AccessorID == aclsOffAccessor,
	}

	if self.ExpirationTime != nil {
		token.Expires = *self.ExpirationTime
	}

	return token, nil
}

// Forbidden says the cluster refused a request with 403: the token may not
// do what was asked, or is not a token the cluster knows.
func Forbidden(err error) bool { return answered(err, http.StatusForbidden) }

// Unanswered says a request failed with no status from the cluster: most
// often the cluster or the network to it is down. 502, 503 and 504 are what a
// proxy in front of the cluster answers when the cluster behind it does not.
// Servers with no leader, as while they start, answer 500 and cannot answer
// the request until they choose one.
func Unanswered(err error) bool {
	var answer api.UnexpectedResponseError

	if !errors.As(err, &answer) {
		return err != nil
	}

	switch answer.StatusCode() {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	case http.StatusInternalServerError:
		return strings.Contains(answer.Body(), noLeader)
	}

	return false
}

// noLeader is what Nomad says when its servers have no leader.
const noLeader = "No cluster leader"

// answered says the cluster refused a request with this status.
func answered(err error, status int) bool {
	var answer api.UnexpectedResponseError

	return errors.As(err, &answer) && answer.StatusCode() == status
}
