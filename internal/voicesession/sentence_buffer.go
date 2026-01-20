package voicesession

import (
	"log/slog"
	"strings"
	"sync"

	"github.com/jdkato/prose/v2"
)

type SentenceBuffer struct {
	mu      sync.Mutex
	buffer  strings.Builder
	emitted int
	log     *slog.Logger
}

func NewSentenceBuffer(log *slog.Logger) *SentenceBuffer {
	if log == nil {
		log = slog.Default()
	}
	return &SentenceBuffer{log: log}
}

func (sb *SentenceBuffer) Add(delta string) []string {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.buffer.WriteString(delta)
	text := sb.buffer.String()

	sentences, hasIncomplete := sb.segment(text)

	if len(sentences) <= sb.emitted {
		return nil
	}

	var completeCount int
	if hasIncomplete {
		completeCount = len(sentences) - 1
	} else {
		completeCount = len(sentences)
	}

	if completeCount <= sb.emitted {
		return nil
	}

	newComplete := sentences[sb.emitted:completeCount]
	sb.emitted = completeCount

	return newComplete
}

func (sb *SentenceBuffer) Flush() string {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	text := sb.buffer.String()
	if text == "" {
		return ""
	}

	sentences, _ := sb.segment(text)

	if len(sentences) <= sb.emitted {
		sb.buffer.Reset()
		sb.emitted = 0
		return ""
	}

	remaining := strings.Join(sentences[sb.emitted:], " ")
	sb.buffer.Reset()
	sb.emitted = 0

	return strings.TrimSpace(remaining)
}

func (sb *SentenceBuffer) Reset() {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	sb.buffer.Reset()
	sb.emitted = 0
}

func (sb *SentenceBuffer) segment(text string) ([]string, bool) {
	doc, err := prose.NewDocument(text,
		prose.WithTokenization(false),
		prose.WithTagging(false),
		prose.WithExtraction(false),
	)
	if err != nil {
		return nil, false
	}

	sentences := doc.Sentences()
	if len(sentences) == 0 {
		return nil, true
	}

	result := make([]string, len(sentences))
	for i, s := range sentences {
		result[i] = s.Text
	}

	lastSent := sentences[len(sentences)-1]
	hasIncomplete := !endsWithTerminator(lastSent.Text)

	return result, hasIncomplete
}

func endsWithTerminator(s string) bool {
	s = strings.TrimRight(s, " \t\n\r")
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	return last == '.' || last == '!' || last == '?'
}
