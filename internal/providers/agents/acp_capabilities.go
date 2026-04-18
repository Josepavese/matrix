package agents

func supportsLoadSession(resp *acpInitializeResponse) bool {
	if resp == nil || resp.Capabilities == nil {
		return false
	}
	enabled, _ := resp.Capabilities["loadSession"].(bool)
	return enabled
}

func supportsSessionCapability(resp *acpInitializeResponse, name string) bool {
	if resp == nil || resp.Capabilities == nil {
		return false
	}
	caps, _ := resp.Capabilities["sessionCapabilities"].(map[string]interface{})
	if caps == nil {
		return false
	}
	return capabilityEnabled(caps[name])
}

func capabilityEnabled(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case map[string]interface{}:
		return value != nil
	default:
		return false
	}
}
