package barertc

type Message struct {
	Action   string `json:"action,omitempty"`
	Channel  string `json:"channel,omitempty"`
	Username string `json:"username,omitempty"`
	Message  string `json:"message,omitempty"`

	// JWT token for `login` actions.
	JWTToken string `json:"jwt,omitempty"`

	// WhoList for `who` actions
	WhoList []WhoList `json:"whoList,omitempty"`

	// Sent on `me` actions along with Username
	VideoActive bool `json:"videoActive,omitempty"` // user tells us their cam status

	// Sent on `open` actions along with the (other) Username.
	OpenSecret string `json:"openSecret,omitempty"`

	// Parameters sent on WebRTC signaling messages.
	Candidate   map[string]interface{} `json:"candidate,omitempty"`   // candidate
	Description map[string]interface{} `json:"description,omitempty"` // sdp
}

const (
	// Actions sent by the client side only
	ActionLogin = "login" // post the username to backend

	// Actions sent by server or client
	ActionMessage = "message" // post a message to the room
	ActionMe      = "me"      // user self-info sent by FE or BE
	ActionOpen    = "open"    // user wants to view a webcam (open WebRTC)
	ActionRing    = "ring"    // receiver of a WebRTC open request

	// Actions sent by server only
	ActionPing     = "ping"
	ActionWhoList  = "who"      // server pushes the Who List
	ActionPresence = "presence" // a user joined or left the room
	ActionError    = "error"    // ChatServer errors

	// WebRTC signaling messages.
	ActionCandidate = "candidate"
	ActionSDP       = "sdp"
)

// WhoList is a member entry in the chat room.
type WhoList struct {
	Username    string `json:"username"`
	VideoActive bool   `json:"videoActive"`

	// JWT auth extra settings.
	Operator   bool   `json:"op"`
	Avatar     string `json:"avatar"`
	ProfileURL string `json:"profileURL"`
}
