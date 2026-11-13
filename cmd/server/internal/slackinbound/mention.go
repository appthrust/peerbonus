package slackinbound

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	mentionToken = regexp.MustCompile(`^<@([UW][A-Z0-9]+)(?:\|[^>]*)?>$`)
	slackMarkup  = regexp.MustCompile(`<[^>]*>`)
	pointToken   = regexp.MustCompile(`^([0-9]+)(?:pts?|ポイント)?$`)
	slackEscapes = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">")
)

func parseMention(text, botUserID string) (recipients []string, points *int, message string) {
	var parts []string
	appendText := func(text string) {
		for _, token := range strings.Fields(text) {
			if match := pointToken.FindStringSubmatch(token); points == nil && match != nil {
				// Atoi saturates on overflow; the store rejects that out-of-range amount.
				amount, _ := strconv.Atoi(match[1])
				points = &amount
			} else {
				parts = append(parts, token)
			}
		}
	}
	start := 0
	for _, span := range slackMarkup.FindAllStringIndex(text, -1) {
		appendText(text[start:span[0]])
		token := text[span[0]:span[1]]
		if match := mentionToken.FindStringSubmatch(token); match != nil {
			if match[1] != botUserID && !slices.Contains(recipients, match[1]) {
				recipients = append(recipients, match[1])
			}
		} else {
			link := token[1 : len(token)-1]
			if _, label, ok := strings.Cut(link, "|"); ok {
				link = label
			}
			parts = append(parts, link)
		}
		start = span[1]
	}
	appendText(text[start:])
	message = strings.Join(strings.Fields(slackEscapes.Replace(strings.Join(parts, " "))), " ")
	return
}
