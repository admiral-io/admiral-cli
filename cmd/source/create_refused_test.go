package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.admiral.io/cli/internal/cmderr"
)

// A member who may not create a source is answered NotFound by the API, so
// the CLI says who can.
func TestCreateRefusedSaysWhoCan(t *testing.T) {
	notFound := status.Error(codes.NotFound, "not found")
	assert.Contains(t, cmderr.Hint(createRefused(notFound, false)), "An organization admin can create it, or grant you source creation.")
	assert.Empty(t, cmderr.Hint(createRefused(notFound, true)), "a named integration may be what was not found")
	assert.Empty(t, cmderr.Hint(createRefused(status.Error(codes.AlreadyExists, "taken"), false)))
}
