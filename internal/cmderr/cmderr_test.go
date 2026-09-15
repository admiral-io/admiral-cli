package cmderr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type hinted struct{}

func (hinted) Error() string { return "boom" }
func (hinted) Hint() string  { return "try again" }

func TestCodes(t *testing.T) {
	require.Equal(t, ExitError, Code(errors.New("x")))
	require.Equal(t, ExitUsage, Code(Usage("bad")))
	require.Equal(t, ExitAuth, Code(Auth("login", errors.New("401"))))
	require.Equal(t, ExitTimeout, Code(Timeout("slow")))
	require.Equal(t, ExitUsage, Code(fmt.Errorf("wrapped: %w", Usage("bad"))), "codes survive wrapping")
}

func TestHints(t *testing.T) {
	require.Equal(t, "", Hint(errors.New("x")))
	require.Equal(t, "h", Hint(UsageHint("h", "bad")))
	require.Equal(t, "try again", Hint(hinted{}), "Hinter errors supply their own hint")
	require.Equal(t, "try again", Hint(fmt.Errorf("ctx: %w", hinted{})))
	w := WithHint(Usage("bad"), "fix")
	require.Equal(t, ExitUsage, Code(w))
	require.Equal(t, "fix", Hint(w))
	require.Nil(t, WithHint(nil, "x"))
}
