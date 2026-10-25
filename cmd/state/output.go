package state

import (
	"strconv"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.admiral.io/cli/internal/client"
	"go.admiral.io/cli/internal/output"
)

var versionTable = output.Table[*client.StateVersion]{
	{Header: "SERIAL", Cell: func(v *client.StateVersion) string { return strconv.FormatInt(v.Serial, 10) }},
	{Header: "WRITER", Cell: writer},
	{Header: "SIZE", Cell: func(v *client.StateVersion) string { return size(v.SizeBytes) }},
	{Header: "FORCED", Cell: func(v *client.StateVersion) string {
		if v.Force {
			return "yes"
		}
		return ""
	}},
	{Header: "AGE", Cell: func(v *client.StateVersion) string { return output.FormatAge(timestamppb.New(v.CreatedAt)) }},
	{Header: "ID", Wide: true, Cell: func(v *client.StateVersion) string { return v.ID }},
}

// writer names who wrote a version: a person's subject, or a job and its
// attempt.
func writer(v *client.StateVersion) string {
	switch {
	case v.Writer.Person != nil:
		return v.Writer.Person.Subject
	case v.Writer.Job != nil:
		return v.Writer.Job.JobID + " (attempt " + strconv.Itoa(int(v.Writer.Job.Attempt)) + ")"
	}
	return output.None
}

// size is a state's size in the largest unit that keeps it at least 1.
func size(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64) + " " + string("KMGT"[exp]) + "iB"
}
