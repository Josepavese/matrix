package agentinstall

// EnvSlice converts a registry environment map to command environment entries.
func EnvSlice(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	return env
}
