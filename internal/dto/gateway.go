package dto

type LiveKitTokenResponse struct {
	Token    string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	URL      string `json:"url" example:"wss://livekit.maix.ovh"`
	Room     string `json:"room" example:"room_abc123xyz"`
	Identity string `json:"identity" example:"user_abc123"`
}
