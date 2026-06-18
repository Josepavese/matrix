package daemon

import (
	"github.com/Josepavese/matrix/internal/logic/vault"
)

// VaultService is the RPC wrapper around the Vault SSOT logic
type VaultService struct {
	v      *vault.Vault
	apiKey string
}

// NewVaultService creates a new RPC service for the Vault
func NewVaultService(v *vault.Vault, apiKey string) *VaultService {
	return &VaultService{v: v, apiKey: apiKey}
}

// VaultArgs represents input for Vault RPC calls
type VaultArgs struct {
	Key    string `json:"key"`
	Value  string `json:"value,omitempty"`
	APIKey string `json:"api_key,omitempty"`
}

// VaultReply represents the output for Vault RPC calls
type VaultReply struct {
	Value string `json:"value"`
}

// Get retrieves a string from the vault over RPC
func (s *VaultService) Get(args *VaultArgs, reply *VaultReply) error {
	if err := s.authorize(args); err != nil {
		return err
	}
	val, err := s.v.GetString(args.Key)
	if err != nil {
		return err
	}
	reply.Value = val
	return nil
}

// Set stores a string into the vault over RPC
func (s *VaultService) Set(args *VaultArgs, _ *VaultReply) error {
	if err := s.authorize(args); err != nil {
		return err
	}
	return s.v.SetString(args.Key, args.Value)
}

func (s *VaultService) authorize(args *VaultArgs) error {
	if s.apiKey == "" {
		return nil
	}
	if args == nil || args.APIKey != s.apiKey {
		return ErrInvalidAPIKey
	}
	return nil
}
