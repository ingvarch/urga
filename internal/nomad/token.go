package nomad

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/hashicorp/nomad/api"
)

// Token is the ACL token the session sends, as the cluster sees it.
type Token struct {
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

// The accessor IDs the cluster answers with when there is no token of its
// own to speak of.
const (
	anonymousAccessor = "anonymous"
	aclsOffAccessor   = "acls-disabled"
)

// Token asks the cluster whose token the session sends. A token it refuses
// is an answer, not a failure to ask.
func (c *Client) Token(ctx context.Context) (Token, error) {
	self, _, err := c.api.ACLTokens().Self(c.query(ctx, ""))
	if Forbidden(err) {
		return Token{Refused: true}, nil
	}

	if err != nil {
		return Token{}, err
	}

	token := Token{
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

// Forbidden says the cluster answered a request with no: the token may not
// do what was asked, or is not a token the cluster knows.
func Forbidden(err error) bool {
	var answer api.UnexpectedResponseError

	return errors.As(err, &answer) && answer.StatusCode() == http.StatusForbidden
}
