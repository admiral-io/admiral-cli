package util

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// labelFlag
// ---------------------------------------------------------------------------

func TestLabelFlag_Type(t *testing.T) {
	var vals []string
	f := &labelFlag{values: &vals}
	require.Equal(t, "string", f.Type())
}

func TestLabelFlag_Set(t *testing.T) {
	var vals []string
	f := &labelFlag{values: &vals}

	require.NoError(t, f.Set("a=1"))
	require.NoError(t, f.Set("b=2"))
	require.Equal(t, []string{"a=1", "b=2"}, vals)
}

func TestLabelFlag_String(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		f := &labelFlag{}
		require.Equal(t, "", f.String())
	})

	t.Run("empty", func(t *testing.T) {
		vals := []string{}
		f := &labelFlag{values: &vals}
		require.Equal(t, "", f.String())
	})

	t.Run("values", func(t *testing.T) {
		vals := []string{"a=1", "b=2"}
		f := &labelFlag{values: &vals}
		require.Equal(t, "a=1, b=2", f.String())
	})
}

// ---------------------------------------------------------------------------
// AddLabelFlag
// ---------------------------------------------------------------------------

func TestAddLabelFlag(t *testing.T) {
	var labels []string
	cmd := &cobra.Command{Use: "test"}
	AddLabelFlag(cmd, &labels, "test usage")

	f := cmd.Flags().Lookup("label")
	require.NotNil(t, f)
	require.Equal(t, "string", f.Value.Type())
	require.Equal(t, "test usage", f.Usage)
}

func TestAddLabelFlag_Repeatable(t *testing.T) {
	var labels []string
	cmd := &cobra.Command{Use: "test", RunE: func(cmd *cobra.Command, args []string) error { return nil }}
	AddLabelFlag(cmd, &labels, "usage")

	cmd.SetArgs([]string{"--label", "a=1", "--label", "b=2"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, []string{"a=1", "b=2"}, labels)
}

// ---------------------------------------------------------------------------
// ParseLabels
// ---------------------------------------------------------------------------

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    map[string]string
		wantErr string
	}{
		{
			name:  "empty",
			input: nil,
			want:  map[string]string{},
		},
		{
			name:  "single",
			input: []string{"env=prod"},
			want:  map[string]string{"env": "prod"},
		},
		{
			name:  "multiple",
			input: []string{"env=prod", "region=us-east-1"},
			want:  map[string]string{"env": "prod", "region": "us-east-1"},
		},
		{
			name:  "value with equals",
			input: []string{"config=key=value"},
			want:  map[string]string{"config": "key=value"},
		},
		{
			name:  "empty value",
			input: []string{"tag="},
			want:  map[string]string{"tag": ""},
		},
		{
			name:    "missing equals",
			input:   []string{"noequalssign"},
			wantErr: `invalid label format "noequalssign": expected key=value`,
		},
		{
			name:  "duplicate key last wins",
			input: []string{"env=dev", "env=prod"},
			want:  map[string]string{"env": "prod"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseLabels(tc.input)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// BuildLabelFilter
// ---------------------------------------------------------------------------

func TestBuildLabelFilter(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		want    string
		wantErr string
	}{
		{
			name:  "empty",
			input: nil,
			want:  "",
		},
		{
			name:  "single",
			input: []string{"region=us-east-1"},
			want:  `field['labels.region'] = 'us-east-1'`,
		},
		{
			name:  "multiple joined with AND",
			input: []string{"region=us-east-1", "env=prod"},
			want:  `field['labels.region'] = 'us-east-1' AND field['labels.env'] = 'prod'`,
		},
		{
			name:    "invalid format",
			input:   []string{"badlabel"},
			wantErr: `invalid label format "badlabel": expected key=value`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildLabelFilter(tc.input)
			if tc.wantErr != "" {
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Help text integration: --label should show as "string" not "stringArray"
// ---------------------------------------------------------------------------

func TestAddLabelFlag_HelpShowsString(t *testing.T) {
	var labels []string
	cmd := &cobra.Command{Use: "test"}
	AddLabelFlag(cmd, &labels, "set a label")

	got := cmd.Flags().FlagUsages()
	require.Contains(t, got, "--label string")
	require.NotContains(t, got, "stringArray")
}
