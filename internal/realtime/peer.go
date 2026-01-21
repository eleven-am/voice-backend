package realtime

import (
	"crypto/rand"
	"encoding/binary"
	"log"
	"strings"
	"sync"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type Peer struct {
	pc         *webrtc.PeerConnection
	audioTrack *webrtc.TrackLocalStaticRTP

	mu             sync.RWMutex
	seq            uint16
	timestamp      uint32
	ssrc           uint32
	onAudio        func([]byte)
	onVideo        func([]byte, string)
	onConnected    func()
	onFailed       func()
	videoTrack     *webrtc.TrackRemote
	videoTrackKind string
}

func NewPeer(pc *webrtc.PeerConnection) (*Peer, error) {
	var ssrcBytes [4]byte
	if _, err := rand.Read(ssrcBytes[:]); err != nil {
		return nil, err
	}
	ssrc := binary.BigEndian.Uint32(ssrcBytes[:])

	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		"audio",
		"voice-audio",
	)
	if err != nil {
		return nil, err
	}

	if _, err := pc.AddTrack(track); err != nil {
		return nil, err
	}

	p := &Peer{
		pc:         pc,
		audioTrack: track,
		ssrc:       ssrc,
	}

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		codec := remoteTrack.Codec()
		log.Printf("OnTrack received: kind=%s codec=%s channels=%d rate=%d fmtp=%s streamID=%s",
			remoteTrack.Kind().String(), codec.MimeType, codec.Channels, codec.ClockRate, codec.SDPFmtpLine, remoteTrack.StreamID())
		if remoteTrack.Kind() == webrtc.RTPCodecTypeAudio {
			go p.readIncomingAudio(remoteTrack)
		} else if remoteTrack.Kind() == webrtc.RTPCodecTypeVideo {
			p.handleVideoTrack(remoteTrack)
		}
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		p.mu.RLock()
		onConnected := p.onConnected
		onFailed := p.onFailed
		p.mu.RUnlock()

		switch state {
		case webrtc.PeerConnectionStateConnected:
			if onConnected != nil {
				onConnected()
			}
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateDisconnected:
			if onFailed != nil {
				onFailed()
			}
		}
	})

	return p, nil
}

func (p *Peer) handleVideoTrack(track *webrtc.TrackRemote) {
	streamID := track.StreamID()
	isScreenShare := streamID == "screen" || streamID == "display" || streamID == "screenshare"

	p.mu.Lock()
	currentKind := p.videoTrackKind
	shouldReplace := p.videoTrack == nil ||
		(isScreenShare && currentKind != "screen") ||
		(!isScreenShare && currentKind == "")

	if shouldReplace {
		p.videoTrack = track
		if isScreenShare {
			p.videoTrackKind = "screen"
		} else {
			p.videoTrackKind = "camera"
		}
		log.Printf("Video track selected: kind=%s streamID=%s", p.videoTrackKind, streamID)
		p.mu.Unlock()
		go p.readIncomingVideo(track)
	} else {
		p.mu.Unlock()
		log.Printf("Video track ignored (preferring %s): streamID=%s", currentKind, streamID)
	}
}

func (p *Peer) readIncomingVideo(track *webrtc.TrackRemote) {
	log.Printf("readIncomingVideo started for %s", track.StreamID())
	buf := make([]byte, 65535)
	packetCount := 0

	for {
		n, _, err := track.Read(buf)
		if err != nil {
			log.Printf("readIncomingVideo error: %v", err)
			return
		}

		packetCount++
		if packetCount <= 5 || packetCount%500 == 0 {
			log.Printf("readIncomingVideo: received packet %d, size=%d", packetCount, n)
		}

		p.mu.RLock()
		cb := p.onVideo
		p.mu.RUnlock()

		if cb != nil {
			if packetCount <= 5 {
				log.Printf("VISION DEBUG peer: calling onVideo callback, raw packet size=%d, mime=%s", n, track.Codec().MimeType)
			}
			data := make([]byte, n)
			copy(data, buf[:n])
			cb(data, track.Codec().MimeType)
		} else if packetCount <= 5 {
			log.Printf("VISION DEBUG peer: onVideo callback is nil!")
		}
	}
}

func (p *Peer) readIncomingAudio(track *webrtc.TrackRemote) {
	log.Printf("readIncomingAudio started")
	buf := make([]byte, 1500)
	packetCount := 0
	for {
		n, _, err := track.Read(buf)
		if err != nil {
			log.Printf("readIncomingAudio error: %v", err)
			return
		}

		packetCount++
		if packetCount <= 5 || packetCount%100 == 0 {
			log.Printf("readIncomingAudio: received packet %d, size=%d", packetCount, n)
		}

		p.mu.RLock()
		cb := p.onAudio
		p.mu.RUnlock()

		if cb != nil {
			pkt := &rtp.Packet{}
			if err := pkt.Unmarshal(buf[:n]); err == nil {
				cb(pkt.Payload)
			}
		} else if packetCount <= 5 {
			log.Printf("readIncomingAudio: no callback set")
		}
	}
}

func (p *Peer) SetOffer(sdp string) error {
	log.Printf("SDP Offer received, checking for video: hasVideo=%v", strings.Contains(sdp, "m=video"))
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}
	return p.pc.SetRemoteDescription(offer)
}

func (p *Peer) CreateAnswer() (string, error) {
	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}
	return answer.SDP, nil
}

func (p *Peer) WriteRTP(opusData []byte, samples int) error {
	p.mu.Lock()
	seq := p.seq
	ts := p.timestamp
	p.seq++
	p.timestamp += uint32(samples)
	p.mu.Unlock()

	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    111,
			SequenceNumber: seq,
			Timestamp:      ts,
			SSRC:           p.ssrc,
		},
		Payload: opusData,
	}

	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	_, err = p.audioTrack.Write(data)
	return err
}

func (p *Peer) AddICECandidate(candidate webrtc.ICECandidateInit) error {
	return p.pc.AddICECandidate(candidate)
}

func (p *Peer) OnAudio(fn func([]byte)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onAudio = fn
}

func (p *Peer) OnVideo(fn func([]byte, string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onVideo = fn
}

func (p *Peer) HasVideoTrack() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.videoTrack != nil
}

func (p *Peer) OnConnected(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onConnected = fn
}

func (p *Peer) OnFailed(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onFailed = fn
}

func (p *Peer) OnICECandidate(fn func(*webrtc.ICECandidate)) {
	p.pc.OnICECandidate(fn)
}

func (p *Peer) OnDataChannel(fn func(*webrtc.DataChannel)) {
	p.pc.OnDataChannel(fn)
}

func (p *Peer) Close() error {
	return p.pc.Close()
}
