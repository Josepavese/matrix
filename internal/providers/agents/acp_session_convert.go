package agents

import "github.com/Josepavese/matrix/internal/middleware"

func fromZedACPSession(resp *acpNewSessionResponse) *middleware.NewSessionResponse {
	if resp == nil {
		return nil
	}
	out := &middleware.NewSessionResponse{SessionID: resp.SessionID}
	applyZedACPSessionState(out, resp.Modes, resp.ConfigOptions)
	return out
}

func fromZedACPResumeSession(resp *acpResumeSessionResponse) *middleware.NewSessionResponse {
	if resp == nil {
		return nil
	}
	out := &middleware.NewSessionResponse{}
	applyZedACPSessionState(out, resp.Modes, resp.ConfigOptions)
	return out
}

func fromZedACPLoadSession(resp *acpLoadSessionResponse) *middleware.NewSessionResponse {
	if resp == nil {
		return nil
	}
	out := &middleware.NewSessionResponse{}
	applyZedACPSessionState(out, resp.Modes, resp.ConfigOptions)
	return out
}

func applyZedACPSessionState(out *middleware.NewSessionResponse, modes *acpSessionModeState, configOptions []acpConfigOption) {
	if out == nil {
		return
	}
	if modes != nil {
		out.Modes = &middleware.SessionModeState{
			CurrentModeID:  modes.CurrentModeID,
			AvailableModes: make([]middleware.SessionMode, 0, len(modes.AvailableModes)),
		}
		for _, mode := range modes.AvailableModes {
			out.Modes.AvailableModes = append(out.Modes.AvailableModes, middleware.SessionMode{
				ID:          mode.ID,
				Name:        mode.Name,
				Description: mode.Description,
			})
		}
	}
	if len(configOptions) > 0 {
		out.ConfigOptions = make([]middleware.ConfigOption, 0, len(configOptions))
		for _, opt := range configOptions {
			converted := middleware.ConfigOption{
				ID:          opt.ID,
				Name:        opt.Name,
				Description: opt.Description,
				Category:    opt.Category,
				Type:        opt.Type,
				Current:     opt.Current,
				Options:     make([]middleware.ConfigOptionValue, 0, len(opt.Options)),
			}
			for _, value := range opt.Options {
				converted.Options = append(converted.Options, middleware.ConfigOptionValue{
					ID:          value.ID,
					Name:        value.Name,
					Description: value.Description,
				})
			}
			out.ConfigOptions = append(out.ConfigOptions, converted)
		}
	}
}
