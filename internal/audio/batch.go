package audio

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/eleven-am/voice-backend/internal/transcription"
	"github.com/eleven-am/voice-backend/internal/transcription/sttpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	transcriptionTimeout  = 60 * time.Second
	defaultMaxMessageSize = 512 * 1024 * 1024
)

type BatchTranscribeRequest struct {
	Filename              string
	AudioData             []byte
	Language              string
	ModelID               string
	Task                  string
	IncludeWordTimestamps bool
}

type BatchTranscribeResult struct {
	Text                 string
	AudioDurationMs      uint64
	ProcessingDurationMs uint64
	Segments             []*sttpb.Segment
	Words                []*sttpb.TranscriptWord
	Model                string
}

func BatchTranscribe(ctx context.Context, cfg transcription.Config, req BatchTranscribeRequest) (*BatchTranscribeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, transcriptionTimeout)
	defer cancel()

	var creds grpc.DialOption
	if cfg.TLSCreds != nil {
		creds = grpc.WithTransportCredentials(cfg.TLSCreds)
	} else {
		creds = grpc.WithTransportCredentials(insecure.NewCredentials())
	}

	maxMsgSize := cfg.MaxMessageSize
	if maxMsgSize <= 0 {
		maxMsgSize = defaultMaxMessageSize
	}

	conn, err := grpc.NewClient(cfg.Address, creds,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMsgSize),
			grpc.MaxCallSendMsgSize(maxMsgSize),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("dial sidecar: %w", err)
	}
	defer conn.Close()

	md := metadata.MD{}
	if cfg.Token != "" {
		md.Set("authorization", fmt.Sprintf("Bearer %s", cfg.Token))
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	client := sttpb.NewTranscriptionServiceClient(conn)
	stream, err := client.Transcribe(ctx)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}

	sessionCfg := &sttpb.SessionConfig{
		Language:              req.Language,
		ModelId:               req.ModelID,
		Partials:              false,
		SampleRate:            16000,
		Task:                  req.Task,
		IncludeWordTimestamps: req.IncludeWordTimestamps,
	}

	if err := stream.Send(&sttpb.ClientMessage{Msg: &sttpb.ClientMessage_Config{Config: sessionCfg}}); err != nil {
		return nil, fmt.Errorf("send config: %w", err)
	}

	if err := stream.Send(&sttpb.ClientMessage{Msg: &sttpb.ClientMessage_EncodedAudio{EncodedAudio: &sttpb.EncodedAudio{
		Format: req.Filename,
		Data:   req.AudioData,
	}}}); err != nil {
		return nil, fmt.Errorf("send audio: %w", err)
	}

	if err := stream.Send(&sttpb.ClientMessage{Msg: &sttpb.ClientMessage_EndOfStream{EndOfStream: true}}); err != nil {
		return nil, fmt.Errorf("send end: %w", err)
	}

	if err := stream.CloseSend(); err != nil {
		return nil, fmt.Errorf("close send: %w", err)
	}

	var result *BatchTranscribeResult

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return result, fmt.Errorf("transcription timeout: %w", ctx.Err())
			}
			return result, fmt.Errorf("recv: %w", err)
		}

		switch m := msg.Msg.(type) {
		case *sttpb.ServerMessage_Transcript:
			t := m.Transcript
			if !t.IsPartial {
				result = &BatchTranscribeResult{
					Text:                 t.Text,
					AudioDurationMs:      t.AudioDurationMs,
					ProcessingDurationMs: t.ProcessingDurationMs,
					Segments:             t.Segments,
					Words:                t.Words,
					Model:                t.Model,
				}
			}
		case *sttpb.ServerMessage_Error:
			return nil, fmt.Errorf("sidecar error: %s", m.Error.Message)
		}
	}

	if result == nil {
		return nil, fmt.Errorf("no transcript received")
	}

	return result, nil
}
