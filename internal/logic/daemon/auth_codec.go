package daemon

import "fmt"

var ErrInvalidAPIKey = fmt.Errorf("invalid API key")

// AuthService provides RPC-based authentication for the JSON-RPC daemon.
// When enabled (via WithAPIKey), clients must call Auth.Authenticate with
// the correct API key before invoking other RPC methods.
type AuthService struct {
	apiKey string
}

// AuthArgs holds the API key sent by the client.
type AuthArgs struct {
	APIKey string `json:"api_key"`
}

// AuthReply holds the authentication result.
type AuthReply struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// Authenticate validates the provided API key.
func (s *AuthService) Authenticate(args *AuthArgs, reply *AuthReply) error {
	if args != nil && args.APIKey == s.apiKey {
		reply.Success = true
		reply.Message = "authenticated"
		return nil
	}
	reply.Success = false
	return ErrInvalidAPIKey
}
