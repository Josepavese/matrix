package zedacp

import (
	"encoding/json"
	"path/filepath"
)

// ErrCodeInvalidParams is JSON-RPC's invalid method parameters error.
const ErrCodeInvalidParams = -32602

// NewInvalidParamsError rejects malformed values without echoing private input.
func NewInvalidParamsError(field string) error {
	return &RPCError{Code: ErrCodeInvalidParams, Message: "invalid request field: " + field}
}

// ValidateClientRequest checks values a receiver will act on before permissions,
// filesystem writes or process creation. Optional display metadata is separate.
func ValidateClientRequest(method string, params json.RawMessage, version int) error {
	kind := map[string]string{
		"fs/read_text_file": "read", "fs/write_text_file": "write", "terminal/create": "create",
		"terminal/output": "terminal", "terminal/wait_for_exit": "terminal", "terminal/kill": "terminal", "terminal/release": "terminal",
		"session/request_permission": "permission",
	}[method]
	if kind == "" {
		return nil
	}
	if version >= ProtocolVersionV2 && kind != "permission" {
		return NewMethodNotFoundError(method)
	}
	fields, err := requestObject(params)
	if err != nil {
		return err
	}
	if err = requireRequestString(fields, "sessionId", false); err != nil {
		return err
	}
	validators := map[string]func(map[string]json.RawMessage) error{
		"read": validateReadRequest, "write": validateWriteRequest, "create": validateTerminalRequest,
		"terminal":   func(f map[string]json.RawMessage) error { return requireRequestString(f, "terminalId", false) },
		"permission": func(f map[string]json.RawMessage) error { return validatePermissionRequest(f, version) },
	}
	return validators[kind](fields)
}
func validateReadRequest(fields map[string]json.RawMessage) error {
	if err := validateRequestPath(fields, "path", true); err != nil {
		return err
	}
	for _, key := range []string{"line", "limit"} {
		if err := validateRequestUint(fields, key, true); err != nil {
			return err
		}
	}
	return nil
}
func validateWriteRequest(fields map[string]json.RawMessage) error {
	if err := validateRequestPath(fields, "path", true); err != nil {
		return err
	}
	return requireRequestString(fields, "content", true)
}

func requestObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, NewInvalidParamsError("params")
	}
	return fields, nil
}

func requireRequestString(fields map[string]json.RawMessage, key string, allowEmpty bool) error {
	raw, ok := fields[key]
	var value string
	if !ok || isJSONNull(raw) || json.Unmarshal(raw, &value) != nil || (!allowEmpty && value == "") {
		return NewInvalidParamsError(key)
	}
	return nil
}

func validateRequestPath(fields map[string]json.RawMessage, key string, required bool) error {
	raw, ok := fields[key]
	if !required && (!ok || isJSONNull(raw)) {
		return nil
	}
	if err := requireRequestString(fields, key, false); err != nil {
		return err
	}
	var value string
	_ = json.Unmarshal(raw, &value)
	if !filepath.IsAbs(value) {
		return NewInvalidParamsError(key)
	}
	return nil
}

func validateRequestUint(fields map[string]json.RawMessage, key string, narrow bool) error {
	raw, ok := fields[key]
	if !ok || isJSONNull(raw) {
		return nil
	}
	var value uint64
	if json.Unmarshal(raw, &value) != nil || (narrow && value > uint64(^uint32(0))) {
		return NewInvalidParamsError(key)
	}
	return nil
}

func validateTerminalRequest(fields map[string]json.RawMessage) error {
	if err := requireRequestString(fields, "command", false); err != nil {
		return err
	}
	if err := validateRequestPath(fields, "cwd", false); err != nil {
		return err
	}
	if err := validateRequestUint(fields, "outputByteLimit", false); err != nil {
		return err
	}
	if err := validateTerminalArgs(fields["args"]); err != nil {
		return err
	}
	return validateTerminalEnv(fields["env"])
}

func validateTerminalArgs(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var args []json.RawMessage
	if isJSONNull(raw) || json.Unmarshal(raw, &args) != nil {
		return NewInvalidParamsError("args")
	}
	for _, arg := range args {
		if err := requireRequestString(map[string]json.RawMessage{"arg": arg}, "arg", true); err != nil {
			return err
		}
	}
	return nil
}
func validateTerminalEnv(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var env []json.RawMessage
	if isJSONNull(raw) || json.Unmarshal(raw, &env) != nil {
		return NewInvalidParamsError("env")
	}
	for _, entry := range env {
		item, err := requestObject(entry)
		if err != nil {
			return err
		}
		if err = requireRequestString(item, "name", true); err != nil {
			return err
		}
		if err = requireRequestString(item, "value", true); err != nil {
			return err
		}
	}
	return nil
}

func validatePermissionRequest(fields map[string]json.RawMessage, version int) error {
	if version >= ProtocolVersionV2 {
		if err := requireRequestString(fields, "title", true); err != nil {
			return err
		}
	} else {
		tool, err := requestObject(fields["toolCall"])
		if err != nil {
			return NewInvalidParamsError("toolCall")
		}
		if err = requireRequestString(tool, "toolCallId", false); err != nil {
			return err
		}
	}
	return validatePermissionOptions(fields["options"])
}

func validatePermissionOptions(raw json.RawMessage) error {
	var options []json.RawMessage
	if len(raw) == 0 || isJSONNull(raw) || json.Unmarshal(raw, &options) != nil {
		return NewInvalidParamsError("options")
	}
	for _, raw := range options {
		option, err := requestObject(raw)
		if err != nil {
			return NewInvalidParamsError("options")
		}
		for _, field := range []string{"optionId", "name", "kind"} {
			if err = requireRequestString(option, field, field == "name"); err != nil {
				return err
			}
		}
	}
	return nil
}
