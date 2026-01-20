package audio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eleven-am/voice-backend/internal/apikey"
	"github.com/eleven-am/voice-backend/internal/shared"
	"github.com/eleven-am/voice-backend/internal/synthesis"
	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/labstack/echo/v4"
)

const (
	maxFileSize      = 500 * 1024 * 1024
	maxInputLength   = 4096
	maxAudioDataSize = 500 * 1024 * 1024
	maxSpeed         = 4.0
	minSpeed         = 0.25
	synthesisTimeout = 30 * time.Second
	initialBufSize   = 64 * 1024
)

var audioBufferPool = sync.Pool{
	New: func() any {
		b := &bytes.Buffer{}
		b.Grow(initialBufSize)
		return b
	},
}

type Handler struct {
	ttsClient   *synthesis.Client
	sttConfig   transcription.Config
	apikeyStore *apikey.Store
	logger      *slog.Logger
}

func NewHandler(ttsClient *synthesis.Client, sttConfig transcription.Config, apikeyStore *apikey.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		ttsClient:   ttsClient,
		sttConfig:   sttConfig,
		apikeyStore: apikeyStore,
		logger:      logger.With("handler", "audio"),
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group) {
	g.POST("/speech", h.HandleSpeech)
	g.POST("/transcriptions", h.HandleTranscriptions)
	g.GET("/voices", h.HandleListVoices)
	g.GET("/models", h.HandleListModels)
}

type SpeechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float32 `json:"speed"`
}

type TranscriptionResponse struct {
	Text string `json:"text"`
}

type TranscriptionVerboseResponse struct {
	Task     string    `json:"task"`
	Language string    `json:"language"`
	Duration float64   `json:"duration"`
	Text     string    `json:"text"`
	Segments []Segment `json:"segments,omitempty"`
	Words    []Word    `json:"words,omitempty"`
}

type Segment struct {
	ID    int     `json:"id"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type Word struct {
	Word  string  `json:"word"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type VoiceResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Language string `json:"language,omitempty"`
	Gender   string `json:"gender,omitempty"`
}

type VoicesListResponse struct {
	Voices []VoiceResponse `json:"voices"`
}

type ModelResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type ModelsListResponse struct {
	Models []ModelResponse `json:"models"`
}

func (h *Handler) validateAPIKey(c echo.Context) (*apikey.APIKey, error) {
	auth := c.Request().Header.Get("Authorization")
	if auth == "" {
		return nil, shared.Unauthorized("missing_auth", "Authorization header required")
	}

	token := strings.TrimPrefix(auth, "Bearer ")
	if token == auth {
		return nil, shared.Unauthorized("invalid_auth", "Invalid authorization format")
	}

	key, err := h.apikeyStore.Validate(c.Request().Context(), token)
	if err != nil {
		return nil, shared.Unauthorized("invalid_key", "Invalid API key")
	}

	return key, nil
}

func (h *Handler) HandleSpeech(c echo.Context) error {
	_, err := h.validateAPIKey(c)
	if err != nil {
		return err
	}

	var req SpeechRequest
	if err := c.Bind(&req); err != nil {
		return shared.BadRequest("invalid_body", "Invalid request body")
	}

	if req.Input == "" {
		return shared.BadRequest("missing_input", "Input text is required")
	}

	if len(req.Input) > maxInputLength {
		return shared.BadRequest("input_too_long", fmt.Sprintf("Input text exceeds maximum length of %d characters", maxInputLength))
	}

	if req.Voice == "" {
		req.Voice = "af_heart"
	}
	if req.ResponseFormat == "" {
		req.ResponseFormat = "mp3"
	}
	if req.Speed < minSpeed || req.Speed > maxSpeed {
		req.Speed = 1.0
	}

	ctx, cancel := context.WithTimeout(c.Request().Context(), synthesisTimeout)
	defer cancel()

	buf := audioBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer audioBufferPool.Put(buf)

	var mu sync.Mutex
	var sampleRate uint32
	var format string
	var audioErr error

	synthReq := synthesis.Request{
		Text:    req.Input,
		VoiceID: req.Voice,
		ModelID: req.Model,
		Speed:   req.Speed,
		Format:  req.ResponseFormat,
	}

	cb := synthesis.Callbacks{
		OnReady: func(sr uint32, voiceID string) {
			mu.Lock()
			sampleRate = sr
			mu.Unlock()
		},
		OnAudio: func(data []byte, f string, sr uint32) {
			mu.Lock()
			defer mu.Unlock()
			if buf.Len()+len(data) > maxAudioDataSize {
				audioErr = fmt.Errorf("audio data exceeds maximum size")
				return
			}
			buf.Write(data)
			format = f
			if sampleRate == 0 {
				sampleRate = sr
			}
		},
		OnDone: func(audioDurationMs, processingDurationMs, textLength uint64) {
			h.logger.Debug("TTS synthesis complete",
				"audio_duration_ms", audioDurationMs,
				"processing_duration_ms", processingDurationMs,
				"text_length", textLength)
		},
		OnError: func(err error) {
			h.logger.Error("TTS synthesis error", "error", err)
		},
	}

	if err := h.ttsClient.Synthesize(ctx, synthReq, cb); err != nil {
		h.logger.Error("synthesis failed", "error", err)
		return shared.InternalError("synthesis_failed", "Speech synthesis failed")
	}

	mu.Lock()
	defer mu.Unlock()

	if audioErr != nil {
		return shared.InternalError("synthesis_failed", audioErr.Error())
	}

	if buf.Len() == 0 {
		return shared.InternalError("synthesis_failed", "No audio data generated")
	}

	contentType := "audio/mpeg"
	switch format {
	case "opus":
		contentType = "audio/opus"
	case "wav":
		contentType = "audio/wav"
	case "pcm":
		contentType = "audio/pcm"
	case "flac":
		contentType = "audio/flac"
	case "mp3":
		contentType = "audio/mpeg"
	}

	c.Response().Header().Set("Content-Type", contentType)
	return c.Blob(http.StatusOK, contentType, buf.Bytes())
}

func (h *Handler) HandleTranscriptions(c echo.Context) error {
	_, err := h.validateAPIKey(c)
	if err != nil {
		return err
	}

	file, err := c.FormFile("file")
	if err != nil {
		return shared.BadRequest("missing_file", "File is required")
	}

	if file.Size > maxFileSize {
		return shared.NewAPIError("file_too_large", "File too large (max 25MB)").ToHTTP(http.StatusRequestEntityTooLarge)
	}

	src, err := file.Open()
	if err != nil {
		return shared.InternalError("file_error", "Failed to open file")
	}
	defer src.Close()

	audioData, err := io.ReadAll(src)
	if err != nil {
		return shared.InternalError("file_error", "Failed to read file")
	}

	model := c.FormValue("model")
	language := c.FormValue("language")
	responseFormat := c.FormValue("response_format")
	if responseFormat == "" {
		responseFormat = "json"
	}

	timestampGranularities := c.Request().Form["timestamp_granularities[]"]
	includeWordTimestamps := false
	for _, g := range timestampGranularities {
		if g == "word" {
			includeWordTimestamps = true
			break
		}
	}

	req := BatchTranscribeRequest{
		Filename:              file.Filename,
		AudioData:             audioData,
		Language:              language,
		ModelID:               model,
		Task:                  "transcribe",
		IncludeWordTimestamps: includeWordTimestamps,
	}

	result, err := BatchTranscribe(c.Request().Context(), h.sttConfig, req)
	if err != nil {
		h.logger.Error("transcription failed", "error", err)
		return shared.InternalError("transcription_failed", "Transcription failed")
	}

	if responseFormat == "text" {
		return c.String(http.StatusOK, result.Text)
	}

	if responseFormat == "verbose_json" {
		segments := make([]Segment, 0, len(result.Segments))
		for i, seg := range result.Segments {
			segments = append(segments, Segment{
				ID:    i,
				Start: float64(seg.Start),
				End:   float64(seg.End),
				Text:  seg.Text,
			})
		}

		words := make([]Word, 0, len(result.Words))
		for _, w := range result.Words {
			words = append(words, Word{
				Word:  w.Word,
				Start: float64(w.Start),
				End:   float64(w.End),
			})
		}

		return c.JSON(http.StatusOK, TranscriptionVerboseResponse{
			Task:     "transcribe",
			Language: language,
			Duration: float64(result.AudioDurationMs) / 1000.0,
			Text:     result.Text,
			Segments: segments,
			Words:    words,
		})
	}

	return c.JSON(http.StatusOK, TranscriptionResponse{
		Text: result.Text,
	})
}

func (h *Handler) HandleListVoices(c echo.Context) error {
	_, err := h.validateAPIKey(c)
	if err != nil {
		return err
	}

	voices, err := h.ttsClient.ListVoices(c.Request().Context())
	if err != nil {
		h.logger.Error("list voices failed", "error", err)
		return shared.InternalError("list_failed", "Failed to list voices")
	}

	resp := VoicesListResponse{
		Voices: make([]VoiceResponse, 0, len(voices)),
	}

	for _, v := range voices {
		resp.Voices = append(resp.Voices, VoiceResponse{
			ID:       v.Id,
			Name:     v.Name,
			Language: v.Language,
			Gender:   v.Gender,
		})
	}

	return c.JSON(http.StatusOK, resp)
}

func (h *Handler) HandleListModels(c echo.Context) error {
	_, err := h.validateAPIKey(c)
	if err != nil {
		return err
	}

	models, err := h.ttsClient.ListModels(c.Request().Context())
	if err != nil {
		h.logger.Error("list models failed", "error", err)
		return shared.InternalError("list_failed", "Failed to list models")
	}

	resp := ModelsListResponse{
		Models: make([]ModelResponse, 0, len(models)),
	}

	for _, m := range models {
		resp.Models = append(resp.Models, ModelResponse{
			ID:          m.Id,
			Name:        m.Name,
			Description: m.Description,
		})
	}

	return c.JSON(http.StatusOK, resp)
}
