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
	synthesisTimeout = 10 * time.Minute
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
	g.POST("/voices", h.HandleCreateVoice)
	g.DELETE("/voices/:voice_id", h.HandleDeleteVoice)
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

type CreateVoiceRequest struct {
	VoiceID       string `form:"voice_id"`
	Name          string `form:"name"`
	Language      string `form:"language"`
	Gender        string `form:"gender"`
	ReferenceText string `form:"reference_text"`
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

// HandleSpeech generates audio from text using text-to-speech
// @Summary      Create speech
// @Description  Generates audio from the input text using the specified voice and model. Supports multiple output formats including mp3, opus, wav, pcm, and flac.
// @Tags         audio
// @Accept       json
// @Produce      audio/mpeg,audio/opus,audio/wav,audio/pcm,audio/flac
// @Param        request body SpeechRequest true "Speech synthesis request"
// @Success      200 {file} binary "Audio data in requested format"
// @Failure      400 {object} shared.APIError "Invalid request (missing input, input too long)"
// @Failure      401 {object} shared.APIError "Unauthorized - invalid or missing API key"
// @Failure      500 {object} shared.APIError "Synthesis failed"
// @Security     APIKeyAuth
// @Router       /audio/speech [post]
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

// HandleTranscriptions transcribes audio to text using speech-to-text
// @Summary      Create transcription
// @Description  Transcribes audio into text using the specified model. Supports multiple response formats (json, text, verbose_json) and optional word-level timestamps.
// @Tags         audio
// @Accept       multipart/form-data
// @Produce      json,text/plain
// @Param        file formData file true "Audio file to transcribe (max 25MB)"
// @Param        model formData string false "Model ID to use for transcription"
// @Param        language formData string false "Language code of the audio (e.g., en, es, fr)"
// @Param        response_format formData string false "Output format: json, text, or verbose_json" default(json)
// @Param        timestamp_granularities[] formData []string false "Timestamp granularities to include (word, segment)"
// @Success      200 {object} TranscriptionResponse "Transcription result (json format)"
// @Success      200 {object} TranscriptionVerboseResponse "Transcription result with timestamps (verbose_json format)"
// @Success      200 {string} string "Plain text transcription (text format)"
// @Failure      400 {object} shared.APIError "Invalid request (missing file)"
// @Failure      401 {object} shared.APIError "Unauthorized - invalid or missing API key"
// @Failure      413 {object} shared.APIError "File too large (max 25MB)"
// @Failure      500 {object} shared.APIError "Transcription failed"
// @Security     APIKeyAuth
// @Router       /audio/transcriptions [post]
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

// HandleListVoices lists all available TTS voices
// @Summary      List voices
// @Description  Returns a list of all available text-to-speech voices with their IDs, names, languages, and genders.
// @Tags         audio
// @Produce      json
// @Success      200 {object} VoicesListResponse "List of available voices"
// @Failure      401 {object} shared.APIError "Unauthorized - invalid or missing API key"
// @Failure      500 {object} shared.APIError "Failed to list voices"
// @Security     APIKeyAuth
// @Router       /audio/voices [get]
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

// HandleListModels lists all available TTS models
// @Summary      List models
// @Description  Returns a list of all available text-to-speech models with their IDs, names, and descriptions.
// @Tags         audio
// @Produce      json
// @Success      200 {object} ModelsListResponse "List of available models"
// @Failure      401 {object} shared.APIError "Unauthorized - invalid or missing API key"
// @Failure      500 {object} shared.APIError "Failed to list models"
// @Security     APIKeyAuth
// @Router       /audio/models [get]
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

const maxVoiceFileSize = 10 * 1024 * 1024

// HandleCreateVoice creates a cloned voice from a reference audio file
// @Summary      Create a cloned voice
// @Description  Creates a new cloned voice from a reference audio file. The voice can then be used for speech synthesis.
// @Tags         audio
// @Accept       multipart/form-data
// @Produce      json
// @Param        file formData file true "Reference audio file (WAV, 1-30s, max 10MB)"
// @Param        voice_id formData string true "Unique voice ID"
// @Param        name formData string false "Display name for the voice"
// @Param        language formData string false "Language code (e.g., English, Chinese)" default(English)
// @Param        gender formData string false "Gender (male/female/neutral)" default(neutral)
// @Param        reference_text formData string true "Transcript of the reference audio"
// @Success      200 {object} VoiceResponse "Created voice"
// @Failure      400 {object} shared.APIError "Invalid request (missing file or voice_id)"
// @Failure      401 {object} shared.APIError "Unauthorized - invalid or missing API key"
// @Failure      413 {object} shared.APIError "File too large (max 10MB)"
// @Failure      500 {object} shared.APIError "Failed to create voice"
// @Security     APIKeyAuth
// @Router       /audio/voices [post]
func (h *Handler) HandleCreateVoice(c echo.Context) error {
	_, err := h.validateAPIKey(c)
	if err != nil {
		return err
	}

	file, err := c.FormFile("file")
	if err != nil {
		return shared.BadRequest("missing_file", "Reference audio file is required")
	}

	if file.Size > maxVoiceFileSize {
		return shared.NewAPIError("file_too_large", "File too large (max 10MB)").ToHTTP(http.StatusRequestEntityTooLarge)
	}

	voiceID := c.FormValue("voice_id")
	if voiceID == "" {
		return shared.BadRequest("missing_voice_id", "voice_id is required")
	}

	referenceText := c.FormValue("reference_text")
	if referenceText == "" {
		return shared.BadRequest("missing_reference_text", "reference_text is required for voice cloning")
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

	name := c.FormValue("name")
	if name == "" {
		name = voiceID
	}
	language := c.FormValue("language")
	if language == "" {
		language = "English"
	}
	gender := c.FormValue("gender")
	if gender == "" {
		gender = "neutral"
	}

	voice, err := h.ttsClient.CreateVoice(c.Request().Context(), voiceID, audioData, name, language, gender, referenceText)
	if err != nil {
		h.logger.Error("create voice failed", "error", err)
		return shared.InternalError("create_failed", "Failed to create voice")
	}

	return c.JSON(http.StatusOK, VoiceResponse{
		ID:       voice.Id,
		Name:     voice.Name,
		Language: voice.Language,
		Gender:   voice.Gender,
	})
}

// HandleDeleteVoice deletes a cloned voice
// @Summary      Delete a cloned voice
// @Description  Deletes a previously cloned voice by its ID.
// @Tags         audio
// @Produce      json
// @Param        voice_id path string true "Voice ID to delete"
// @Success      200 {object} map[string]bool "Success status"
// @Failure      400 {object} shared.APIError "Invalid request (missing voice_id)"
// @Failure      401 {object} shared.APIError "Unauthorized - invalid or missing API key"
// @Failure      404 {object} shared.APIError "Voice not found"
// @Failure      500 {object} shared.APIError "Failed to delete voice"
// @Security     APIKeyAuth
// @Router       /audio/voices/{voice_id} [delete]
func (h *Handler) HandleDeleteVoice(c echo.Context) error {
	_, err := h.validateAPIKey(c)
	if err != nil {
		return err
	}

	voiceID := c.Param("voice_id")
	if voiceID == "" {
		return shared.BadRequest("missing_voice_id", "voice_id is required")
	}

	success, err := h.ttsClient.DeleteVoice(c.Request().Context(), voiceID)
	if err != nil {
		h.logger.Error("delete voice failed", "error", err)
		return shared.InternalError("delete_failed", "Failed to delete voice")
	}

	if !success {
		return shared.NewAPIError("not_found", "Voice not found").ToHTTP(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, map[string]bool{"success": true})
}
