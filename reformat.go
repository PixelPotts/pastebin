package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const reformatPrompt = `You are an aggressive text cleanup tool. You take messy, broken, copy-pasted text and produce clean, readable output suitable for pasting into any text box.

DO:
- Join broken lines that are clearly mid-sentence or mid-paragraph into flowing text.
- Break massive single-line walls of text into logical paragraphs.
- Normalize whitespace: collapse runs of spaces/tabs, fix indentation.
- Remove copy-paste junk: soft hyphens, zero-width chars, non-breaking spaces, BOMs, trailing whitespace.
- Remove UI artifacts that got copied by accident: "REPLY", "SHARE", "LIKE", "Reply", "Show more", "Read more", "View thread", vote counts, timestamps like "2h ago", "· 3d", usernames/handles that aren't part of the content, navigation breadcrumbs, cookie banners, "Sign in", "Subscribe", footer links.
- Remove orphaned/cutoff words or sentence fragments at the very start or end that are clearly truncated from a larger context (a trailing word with no sentence, a dangling "the", etc).
- Remove stray line numbers, bullet artifacts (lone •, -, *), and decorative separators (───, ***, ===) that aren't part of the content.
- Collapse redundant blank lines to a single blank line between paragraphs.

DO NOT:
- Rephrase, reword, paraphrase, or rewrite ANY of the actual content.
- Add your own words, commentary, or markdown formatting.
- Summarize or shorten the actual message.
- Change the author's vocabulary, tone, or meaning in any way.

PRESERVE: paragraph breaks between distinct ideas, code blocks, intentional lists, headings.

Return ONLY the cleaned text. No preamble, no wrapping, no explanation.`

var client anthropic.Client
var clientReady bool

// sanitizeKey strips anything that isn't a printable ASCII character.
func sanitizeKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '!' && r <= '~' { // printable ASCII, no spaces
			b.WriteRune(r)
		}
	}
	return b.String()
}

func initClient(apiKey string) {
	key := sanitizeKey(apiKey)
	if key == "" {
		key = sanitizeKey(os.Getenv("ANTHROPIC_API_KEY"))
	}
	if key == "" {
		log.Println("[reformat] WARNING: no API key found")
		return
	}
	log.Printf("[reformat] API key: %s...%s (%d chars)", key[:10], key[len(key)-4:], len(key))
	client = anthropic.NewClient(option.WithAPIKey(key))
	clientReady = true
}

// ReformatText sends text to Claude Haiku for formatting cleanup.
// Returns the cleaned text, or the original on any error.
func ReformatText(raw string) string {
	if !clientReady {
		log.Println("[reformat] client not ready, returning raw")
		return raw
	}
	if len(raw) < 10 {
		log.Println("[reformat] text too short (<10 chars), returning raw")
		return raw
	}
	log.Printf("[reformat] calling Claude Haiku with %d bytes...", len(raw))

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     "claude-haiku-4-5",
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(raw)),
		},
		System: []anthropic.TextBlockParam{
			{Text: reformatPrompt},
		},
	})
	if err != nil {
		log.Printf("reformat error: %v", err)
		return raw
	}

	if len(resp.Content) > 0 && resp.Content[0].Text != "" {
		log.Printf("[reformat] success, got %d bytes back", len(resp.Content[0].Text))
		return resp.Content[0].Text
	}
	log.Println("[reformat] empty response, returning raw")
	return raw
}
