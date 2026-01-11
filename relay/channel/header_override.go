package channel

import (
	"net/http"
	"net/textproto"
	"strings"

	"github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

type headerOverrideSpec struct {
	Override map[string]string
	Fill     map[string]string
	Remove   []string
}

// buildHeaderOverrideSpec parses header overrides and applies variable replacement.
// Supported variables: {api_key}
func buildHeaderOverrideSpec(info *common.RelayInfo) (*headerOverrideSpec, error) {
	if len(info.HeadersOverride) == 0 {
		return &headerOverrideSpec{}, nil
	}

	if isStructuredHeaderOverride(info.HeadersOverride) {
		override, err := parseHeaderOverrideMap(info.HeadersOverride["override"], info)
		if err != nil {
			return nil, err
		}
		fill, err := parseHeaderOverrideMap(info.HeadersOverride["fill"], info)
		if err != nil {
			return nil, err
		}
		remove, err := parseHeaderOverrideRemoveList(info.HeadersOverride["remove"])
		if err != nil {
			return nil, err
		}
		return &headerOverrideSpec{
			Override: override,
			Fill:     fill,
			Remove:   remove,
		}, nil
	}

	legacyOverride, err := parseHeaderOverrideMap(info.HeadersOverride, info)
	if err != nil {
		return nil, err
	}
	return &headerOverrideSpec{
		Override: legacyOverride,
	}, nil
}

func isStructuredHeaderOverride(raw map[string]any) bool {
	if raw == nil {
		return false
	}
	for _, key := range []string{"override", "fill", "remove"} {
		if value, ok := raw[key]; ok {
			switch value.(type) {
			case map[string]any, map[string]string, []any, []string, nil:
				return true
			default:
				// likely legacy format
			}
		}
	}
	return false
}

func parseHeaderOverrideMap(raw any, info *common.RelayInfo) (map[string]string, error) {
	if raw == nil {
		return map[string]string{}, nil
	}
	switch rawMap := raw.(type) {
	case map[string]any:
		result := make(map[string]string, len(rawMap))
		for k, v := range rawMap {
			str, ok := v.(string)
			if !ok {
				return nil, types.NewError(nil, types.ErrorCodeChannelHeaderOverrideInvalid)
			}
			result[k] = replaceHeaderVariables(str, info)
		}
		return result, nil
	case map[string]string:
		result := make(map[string]string, len(rawMap))
		for k, v := range rawMap {
			result[k] = replaceHeaderVariables(v, info)
		}
		return result, nil
	default:
		return nil, types.NewError(nil, types.ErrorCodeChannelHeaderOverrideInvalid)
	}
}

func parseHeaderOverrideRemoveList(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	switch list := raw.(type) {
	case []string:
		return list, nil
	case []any:
		result := make([]string, 0, len(list))
		for _, item := range list {
			str, ok := item.(string)
			if !ok {
				return nil, types.NewError(nil, types.ErrorCodeChannelHeaderOverrideInvalid)
			}
			result = append(result, str)
		}
		return result, nil
	default:
		return nil, types.NewError(nil, types.ErrorCodeChannelHeaderOverrideInvalid)
	}
}

func replaceHeaderVariables(value string, info *common.RelayInfo) string {
	if strings.Contains(value, "{api_key}") {
		return strings.ReplaceAll(value, "{api_key}", info.ApiKey)
	}
	return value
}

func applyHeaderOverride(headers http.Header, override *headerOverrideSpec) {
	if override == nil {
		return
	}
	for _, key := range override.Remove {
		canonical := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if canonical == "" {
			continue
		}
		headers.Del(canonical)
	}
	for key, value := range override.Fill {
		canonical := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if canonical == "" {
			continue
		}
		if headers.Get(canonical) == "" {
			headers.Set(canonical, value)
		}
	}
	for key, value := range override.Override {
		canonical := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
		if canonical == "" {
			continue
		}
		headers.Set(canonical, value)
	}
}
