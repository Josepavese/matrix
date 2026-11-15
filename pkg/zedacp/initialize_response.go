package zedacp

import "encoding/json"

// UnmarshalJSON reads the parameter names of the generation that answered. A v2
// peer sends capabilities and info; a v1 peer sends agentCapabilities and
// agentInfo. Reading only the v1 names silently dropped everything a v2 peer
// advertised, so Matrix believed a conforming agent supported nothing at all.
//
// The retired spelling stays retired where it was retired: a response declaring
// version 1 must not populate capabilities from that name. Decoding is by
// generation, not by whichever key happens to be present.
func (r *InitializeResponse) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if err := decodeInitializeInto(fields, &r.ProtocolVersion, "protocolVersion"); err != nil {
		return err
	}
	if err := decodeInitializeInto(fields, &r.AuthMethods, "authMethods"); err != nil {
		return err
	}
	if r.ProtocolVersion < ProtocolVersionV2 {
		return decodeInitializeV1(r, fields)
	}
	r.V2PayloadConforming = hasJSONObject(fields, "info") && hasJSONObject(fields, "capabilities")
	return decodeInitializeV2(r, fields)
}

func decodeInitializeV1(r *InitializeResponse, fields map[string]json.RawMessage) error {
	if err := decodeInitializeInto(fields, &r.AgentInfo, "agentInfo"); err != nil {
		return err
	}
	return decodeInitializeInto(fields, &r.Capabilities, "agentCapabilities")
}

func decodeInitializeV2(r *InitializeResponse, fields map[string]json.RawMessage) error {
	if err := decodeInitializeInto(fields, &r.AgentInfo, "info", "agentInfo"); err != nil {
		return err
	}
	return decodeInitializeInto(fields, &r.Capabilities, "capabilities", "agentCapabilities")
}

func decodeInitializeInto(fields map[string]json.RawMessage, target interface{}, keys ...string) error {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		return json.Unmarshal(raw, target)
	}
	return nil
}

// hasJSONObject reports whether a parameter is present as a JSON object. A
// null or missing key is not a conforming version 2 parameter.
func hasJSONObject(fields map[string]json.RawMessage, key string) bool {
	raw, ok := fields[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(raw, &object) == nil
}
