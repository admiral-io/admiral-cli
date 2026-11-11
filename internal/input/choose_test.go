package input

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"go.admiral.io/cli/internal/cmderr"
)

var choices = []Option{{Name: "v1.2.0", Detail: "version"}, {Name: "v1.1.0", Detail: "version"}, {Name: "main", Detail: "branch"}}

func TestChoose_TakesANameOrANumber(t *testing.T) {
	for answer, want := range map[string]string{"1\n": "v1.2.0", "3\n": "main", "v1.1.0\n": "v1.1.0"} {
		cmd, errOut := interactiveCmd(t, answer)
		got, err := Choose(cmd, "--version", "version", choices)
		require.NoError(t, err, answer)
		require.Equal(t, want, got.Name)
		require.Contains(t, errOut.String(), "  3. main  (branch)\n")
		require.Contains(t, errOut.String(), "Choose the version [number or name]: ")
	}
}

func TestChoose_ANameThatLooksLikeANumberIsTheName(t *testing.T) {
	tags := []Option{{Name: "3"}, {Name: "2"}, {Name: "1"}}
	cmd, _ := interactiveCmd(t, "1\n")
	got, err := Choose(cmd, "--version", "version", tags)
	require.NoError(t, err)
	require.Equal(t, "1", got.Name, "the tag named 1, not the first option")
}

func TestChoose_NumbersOnlyWhatItShows(t *testing.T) {
	var many []Option
	for i := range 60 {
		many = append(many, Option{Name: fmt.Sprintf("v%d", i)})
	}
	cmd, errOut := interactiveCmd(t, "v59\n")
	got, err := Choose(cmd, "--version", "version", many)
	require.NoError(t, err)
	require.Equal(t, "v59", got.Name, "an option past the shown ones is still taken by name")
	require.Contains(t, errOut.String(), "  and 10 more, by name\n")
	require.NotContains(t, errOut.String(), "v50")

	cmd, _ = interactiveCmd(t, "51\n")
	_, err = Choose(cmd, "--version", "version", many)
	require.EqualError(t, err, "51 is not between 1 and 50")
}

func TestChoose_NothingIsPreselected(t *testing.T) {
	cmd, errOut := interactiveCmd(t, "\n")
	_, err := Choose(cmd, "--version", "version", choices)
	require.ErrorIs(t, err, ErrCanceled)
	require.Contains(t, errOut.String(), "canceled")
}

func TestChoose_RefusesWhatIsNotListed(t *testing.T) {
	for _, answer := range []string{"0\n", "4\n", "v9\n"} {
		cmd, _ := interactiveCmd(t, answer)
		_, err := Choose(cmd, "--version", "version", choices)
		require.Error(t, err, answer)
		require.Equal(t, cmderr.ExitUsage, cmderr.Code(err), answer)
	}
}

func TestChoose_NotInteractiveNamesTheFlagAndTheOptions(t *testing.T) {
	cmd, _ := pipedCmd("1\n")
	_, err := Choose(cmd, "--version", "version", choices)
	require.EqualError(t, err, "--version required when not running interactively")
	require.Equal(t, cmderr.ExitUsage, cmderr.Code(err))
	require.Equal(t, "Pass --version as one of: v1.2.0, v1.1.0, main", cmderr.Hint(err))

	var many []Option
	for i := range 25 {
		many = append(many, Option{Name: fmt.Sprintf("v%d", i)})
	}
	_, err = Choose(cmd, "--version", "version", many)
	require.Contains(t, cmderr.Hint(err), "v19, and 5 more")
	require.NotContains(t, cmderr.Hint(err), "v20")

	_, err = Choose(cmd, "--version", "version", nil)
	require.EqualError(t, err, "no version to choose from")
}
