package slackinbound

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMention(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		recipients []string
		points     *int
		message    string
	}{
		{"ordinary", "<@UBOT> <@U1> 30 Thanks", []string{"U1"}, new(30), "Thanks"},
		{"order insensitive", "Thanks 30pt <@U1|Alice> <@UBOT>", []string{"U1"}, new(30), "Thanks"},
		{"distinct in order", "<@U2> <@U1> <@U2|Bob> <@UBOT> 10pts Thanks", []string{"U2", "U1"}, new(10), "Thanks"},
		{"Japanese suffix", "<@UBOT> <@U1> 50ポイント ありがとう", []string{"U1"}, new(50), "ありがとう"},
		{"no points", "<@UBOT> <@U1> Thank you", []string{"U1"}, nil, "Thank you"},
		{"word digits", "<@U1> v30 task123 30ptx １２３ Thanks", []string{"U1"}, nil, "v30 task123 30ptx １２３ Thanks"},
		{"first integer only", "<@U1> 10 Thanks for 30 reviews", []string{"U1"}, new(10), "Thanks for 30 reviews"},
		{"escapes and whitespace", "<@UBOT>\n<@U1> 30\tA &amp; B &lt;3 &gt;2", []string{"U1"}, new(30), "A & B <3 >2"},
		{"links", "<@U1> 30 Read <https://example.test/a|the guide> and <https://example.test/b>", []string{"U1"}, new(30), "Read the guide and https://example.test/b"},
		{"link label is not points", "<@U1> <https://example.test/30|30> Thanks", []string{"U1"}, nil, "30 Thanks"},
		{"bot only", "<@UBOT> 30 Thanks", nil, new(30), "Thanks"},
		{"mention adjacency", "<@UBOT><@U1>30 Thanks", []string{"U1"}, new(30), "Thanks"},
		{"escaped mention is text", "<@U1> &lt;@U2&gt; Thanks", []string{"U1"}, nil, "<@U2> Thanks"},
		{"decimal is not integer", "<@U1> 3.5 Thanks", []string{"U1"}, nil, "3.5 Thanks"},
		{"empty", "", nil, nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipients, points, message := parseMention(tc.text, "UBOT")
			require.Equal(t, tc.recipients, recipients)
			require.Equal(t, tc.points, points)
			require.Equal(t, tc.message, message)
		})
	}
}
