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

const reformatPrompt = `You are a text formatting cleanup tool. Your ONLY job is to fix formatting issues in the provided text so it pastes cleanly into any text box.

RULES — follow these exactly:
- Do NOT change, rephrase, reword, or rewrite ANY words or sentences.
- Do NOT add, remove, or summarize any content.
- Do NOT add commentary, explanations, or markdown formatting.
- ONLY fix these formatting problems:
  • Line breaks that split a sentence or paragraph mid-flow — join them.
  • Excessive or inconsistent whitespace — normalize to single spaces.
  • Copy-paste artifacts: stray line numbers, trailing whitespace, soft hyphens, non-breaking spaces, zero-width characters.
  • Redundant blank lines — collapse to a single blank line between paragraphs.
- PRESERVE intentional structure: paragraph breaks, code blocks, lists, headings.
- If the text is already clean, return it unchanged.

Return ONLY the cleaned text. No preamble, no explanation.`

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
