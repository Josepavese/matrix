package agents

import "github.com/Josepavese/matrix/internal/middleware"

func fromZedACPSession(resp *acpNewSessionResponse) *middleware.NewSessionResponse {
	if resp == nil {
		return nil
	}
	out := &middleware.NewSessionResponse{SessionID: resp.SessionID}
	if resp.Modes != nil {
		out.Modes = &middleware.SessionModeState{
			CurrentModeID:  resp.Modes.CurrentModeID,
			AvailableModes: make([]middleware.SessionMode, 0, len(resp.Modes.AvailableModes)),
		}
		for _, mode := range resp.Modes.AvailableModes {
			out.Modes.AvailableModes = append(out.Modes.AvailableModes, middleware.SessionMode{
				ID:          mode.ID,
				Name:        mode.Name,
				Description: mode.Description,
			})
		}
	}
	if len(resp.ConfigOptions) > 0 {
		out.ConfigOptions = make([]middleware.ConfigOption, 0, len(resp.ConfigOptions))
		for _, opt := range resp.ConfigOptions {
			converted := middleware.ConfigOption{
				ID:       opt.ID,
				Name:     opt.Name,
				Category: opt.Category,
				Current:  opt.Current,
				Options:  make([]middleware.ConfigOptionValue, 0, len(opt.Options)),
			}
			for _, value := range opt.Options {
				converted.Options = append(converted.Options, middleware.ConfigOptionValue{
					ID:   value.ID,
					Name: value.Name,
				})
			}
			out.ConfigOptions = append(out.ConfigOptions, converted)
		}
	}
	return out
}
