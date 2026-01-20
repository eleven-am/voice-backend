package realtime

type Config struct {
	ICEServers  []ICEServerConfig
	PortRange   PortRange
	BufferSizes BufferSizes
	MaxSDPSize  int

	TurnServer string
	TurnSecret string
	TurnRealm  string
	TurnTTL    int
}

type ICEServerConfig struct {
	URLs       []string
	Username   string
	Credential string
}

type PortRange struct {
	Min int
	Max int
}

type BufferSizes struct {
	AudioFrames   int
	Events        int
	ICECandidates int
}
