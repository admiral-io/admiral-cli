package output

import (
	"testing"

	commonv1 "buf.build/gen/go/admiral/common/protocolbuffers/go/admiral/common/v1"
	"github.com/stretchr/testify/require"
)

func TestFormatActor(t *testing.T) {
	named := &commonv1.ActorRef{Id: "usr-s0jg9fam3x0x", DisplayName: "Ada Lovelace", Email: "ada@example.com"}
	emailOnly := &commonv1.ActorRef{Id: "usr-s0jg9fam3x0x", Email: "ada@example.com"}
	idOnly := &commonv1.ActorRef{Id: "sa-4k2m9x0q1p7z"}

	require.Equal(t, "Ada Lovelace", FormatActor(named))
	require.Equal(t, "ada@example.com", FormatActor(emailOnly))
	require.Equal(t, "sa-4k2m9x0q1p7z", FormatActor(idOnly))
	require.Equal(t, None, FormatActor(nil))

	require.Equal(t, "Ada Lovelace (usr-s0jg9fam3x0x)", FormatActorWithID(named))
	require.Equal(t, "ada@example.com (usr-s0jg9fam3x0x)", FormatActorWithID(emailOnly))
	require.Equal(t, "sa-4k2m9x0q1p7z", FormatActorWithID(idOnly))
	require.Equal(t, None, FormatActorWithID(nil))
}
