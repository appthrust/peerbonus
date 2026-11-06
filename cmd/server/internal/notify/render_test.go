package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/appthrust/peerbonus/cmd/server/internal/domain"
	"github.com/appthrust/peerbonus/cmd/server/internal/slack"
	"github.com/stretchr/testify/require"
)

func TestGoldenRenders(t *testing.T) {
	sender := domain.Member{ID: "sender", DisplayName: "Alice & Co"}
	recipient := domain.Member{ID: "recipient", DisplayName: "Bob <Ops>"}
	post := domain.Post{ID: "post1", Sender: sender, Recipient: recipient, Points: 30, Message: "Thanks & <cheers>\nGreat work!", Values: []string{"Teamwork", "<Learn>"}, ClapCount: 7}
	claps := []domain.Clap{
		{Member: domain.Member{DisplayName: "C & D"}},
		{Member: domain.Member{DisplayName: "<@UEVIL>"}},
		{Member: domain.Member{DisplayName: "Eve"}},
		{Member: domain.Member{DisplayName: "Omitted fourth"}},
	}
	ids := map[string]string{"sender": "UALICE"}
	for _, locale := range []domain.SlackLocale{domain.SlackLocaleJA, domain.SlackLocaleEN} {
		for _, kind := range []string{"announce", "dm", "clap_update"} {
			t.Run(kind+"_"+string(locale), func(t *testing.T) {
				var message slack.Message
				switch kind {
				case "announce":
					message = Announce(post, sender, recipient, nil, locale, "https://bonus.example.test/", ids)
				case "dm":
					message = DirectMessage(post, sender, recipient, locale, "https://bonus.example.test/", ids)
				case "clap_update":
					message = ClapUpdate(post, sender, recipient, claps, locale, "https://bonus.example.test/", ids)
				}
				expected, err := os.ReadFile(filepath.Join("testdata", kind+"_"+string(locale)+".json"))
				require.NoError(t, err)
				actual, err := json.Marshal(struct {
					Text   string          `json:"text"`
					Blocks json.RawMessage `json:"blocks"`
				}{message.Text, message.Blocks})
				require.NoError(t, err)
				require.JSONEq(t, string(expected), string(actual))
				require.NotEmpty(t, message.Text)
			})
		}
	}
}

func TestRenderEscapingAndOptionalContext(t *testing.T) {
	post := domain.Post{ID: "post1", Message: "<!channel> & <https://evil.test|click>"}
	sender := domain.Member{ID: "sender", DisplayName: "<@UEVIL> & Alice"}
	recipient := domain.Member{ID: "recipient", DisplayName: "Bob"}
	message := Announce(post, sender, recipient, nil, domain.SlackLocaleEN, "https://bonus.example.test", nil)
	require.Contains(t, message.Text, "&lt;@UEVIL&gt; &amp; Alice")
	require.Contains(t, message.Text, "&lt;!channel&gt; &amp; &lt;https://evil.test|click&gt;")
	require.NotContains(t, message.Text, "<!channel>")
	var blocks []struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(message.Blocks, &blocks))
	require.Len(t, blocks, 3)
	require.Equal(t, "actions", blocks[2].Type)
	require.Equal(t, "https://bonus.example.test/posts/a%2Fb", postURL("https://bonus.example.test/", "a/b"))
}
