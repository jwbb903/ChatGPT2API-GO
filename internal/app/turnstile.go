package app

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

type turnstileOrderedMap struct {
	keys   []string
	values map[string]any
}

func newTurnstileOrderedMap() *turnstileOrderedMap {
	return &turnstileOrderedMap{values: map[string]any{}}
}

func (m *turnstileOrderedMap) add(key string, value any) {
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func (m *turnstileOrderedMap) MarshalJSON() ([]byte, error) {
	parts := []string{}
	for _, key := range m.keys {
		kb, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(m.values[key])
		if err != nil {
			return nil, err
		}
		parts = append(parts, string(kb)+":"+string(vb))
	}
	return []byte("{" + strings.Join(parts, ",") + "}"), nil
}

func turnstileToString(value any) string {
	if value == nil {
		return "undefined"
	}
	switch v := value.(type) {
	case float64:
		if math.Trunc(v) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		special := map[string]string{
			"window.Math":            "[object Math]",
			"window.Reflect":         "[object Reflect]",
			"window.performance":     "[object Performance]",
			"window.localStorage":    "[object Storage]",
			"window.Object":          "function Object() { [native code] }",
			"window.Reflect.set":     "function set() { [native code] }",
			"window.performance.now": "function () { [native code] }",
			"window.Object.create":   "function create() { [native code] }",
			"window.Object.keys":     "function keys() { [native code] }",
			"window.Math.random":     "function random() { [native code] }",
		}
		if out, ok := special[v]; ok {
			return out
		}
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return strings.TrimSpace(strAny(value, ""))
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ",")
	default:
		return strings.TrimSpace(strAny(value, ""))
	}
}

func xorString(text, key string) string {
	if key == "" {
		return text
	}
	textRunes := []rune(text)
	keyRunes := []rune(key)
	out := make([]rune, len(textRunes))
	for i, ch := range textRunes {
		out[i] = ch ^ keyRunes[i%len(keyRunes)]
	}
	return string(out)
}

func solveTurnstileToken(dx, p string) string {
	decoded, err := base64.StdEncoding.DecodeString(dx)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(dx)
	}
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(dx)
	}
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(dx)
	}
	if err != nil {
		return ""
	}
	var tokenList []any
	if err := json.Unmarshal([]byte(xorString(string(decoded), p)), &tokenList); err != nil {
		return ""
	}
	processMap := map[int]any{}
	start := time.Now()
	result := ""
	type turnstileFunc func(args ...any)
	index := func(v any) int { return intAny(v, 0) }
	value := func(v any) any { return processMap[index(v)] }
	call := func(fn any, args ...any) {
		if f, ok := fn.(turnstileFunc); ok {
			f(args...)
		}
	}
	processMap[1] = turnstileFunc(func(args ...any) {
		e, t := index(args[0]), index(args[1])
		processMap[e] = xorString(turnstileToString(processMap[e]), turnstileToString(processMap[t]))
	})
	processMap[2] = turnstileFunc(func(args ...any) { processMap[index(args[0])] = args[1] })
	processMap[3] = turnstileFunc(func(args ...any) { result = base64.StdEncoding.EncodeToString([]byte(turnstileToString(args[0]))) })
	processMap[5] = turnstileFunc(func(args ...any) {
		e, t := index(args[0]), index(args[1])
		current, incoming := processMap[e], processMap[t]
		if list, ok := current.([]any); ok {
			processMap[e] = append(list, incoming)
			return
		}
		if _, ok := current.(string); ok {
			processMap[e] = turnstileToString(current) + turnstileToString(incoming)
			return
		}
		if _, ok := current.(float64); ok {
			processMap[e] = turnstileToString(current) + turnstileToString(incoming)
			return
		}
		if _, ok := incoming.(string); ok {
			processMap[e] = turnstileToString(current) + turnstileToString(incoming)
			return
		}
		if _, ok := incoming.(float64); ok {
			processMap[e] = turnstileToString(current) + turnstileToString(incoming)
			return
		}
		processMap[e] = "NaN"
	})
	processMap[6] = turnstileFunc(func(args ...any) {
		e := index(args[0])
		tv, tok := value(args[1]).(string)
		nv, nok := value(args[2]).(string)
		if tok && nok {
			joined := tv + "." + nv
			if joined == "window.document.location" {
				processMap[e] = chatGPTBaseURL + "/"
			} else {
				processMap[e] = joined
			}
		}
	})
	processMap[7] = turnstileFunc(func(args ...any) {
		target := value(args[0])
		values := []any{}
		for _, arg := range args[1:] {
			values = append(values, value(arg))
		}
		if target == "window.Reflect.set" && len(values) >= 3 {
			if obj, ok := values[0].(*turnstileOrderedMap); ok {
				obj.add(turnstileToString(values[1]), values[2])
			}
			return
		}
		call(target, values...)
	})
	processMap[8] = turnstileFunc(func(args ...any) { processMap[index(args[0])] = value(args[1]) })
	processMap[9] = tokenList
	processMap[10] = "window"
	processMap[14] = turnstileFunc(func(args ...any) {
		var parsed any
		if json.Unmarshal([]byte(turnstileToString(value(args[1]))), &parsed) == nil {
			processMap[index(args[0])] = parsed
		}
	})
	processMap[15] = turnstileFunc(func(args ...any) {
		b, _ := json.Marshal(value(args[1]))
		processMap[index(args[0])] = string(b)
	})
	processMap[16] = p
	processMap[17] = turnstileFunc(func(args ...any) {
		e := index(args[0])
		target := value(args[1])
		callArgs := []any{}
		for _, arg := range args[2:] {
			callArgs = append(callArgs, value(arg))
		}
		switch target {
		case "window.performance.now":
			processMap[e] = (float64(time.Since(start).Nanoseconds()) + rand.Float64()) / 1e6
		case "window.Object.create":
			processMap[e] = newTurnstileOrderedMap()
		case "window.Object.keys":
			if len(callArgs) > 0 && callArgs[0] == "window.localStorage" {
				processMap[e] = []any{"STATSIG_LOCAL_STORAGE_INTERNAL_STORE_V4", "STATSIG_LOCAL_STORAGE_STABLE_ID", "client-correlated-secret", "oai/apps/capExpiresAt", "oai-did", "STATSIG_LOCAL_STORAGE_LOGGING_REQUEST", "UiState.isNavigationCollapsed.1"}
			}
		case "window.Math.random":
			processMap[e] = rand.Float64()
		default:
			call(target, callArgs...)
		}
	})
	processMap[18] = turnstileFunc(func(args ...any) {
		if decoded, err := base64.StdEncoding.DecodeString(turnstileToString(value(args[0]))); err == nil {
			processMap[index(args[0])] = string(decoded)
		}
	})
	processMap[19] = turnstileFunc(func(args ...any) {
		processMap[index(args[0])] = base64.StdEncoding.EncodeToString([]byte(turnstileToString(value(args[0]))))
	})
	processMap[20] = turnstileFunc(func(args ...any) {
		if turnstileToString(value(args[0])) == turnstileToString(value(args[1])) {
			callArgs := []any{}
			for _, arg := range args[3:] {
				callArgs = append(callArgs, value(arg))
			}
			call(value(args[2]), callArgs...)
		}
	})
	processMap[21] = turnstileFunc(func(args ...any) {})
	processMap[23] = turnstileFunc(func(args ...any) {
		if value(args[0]) != nil {
			call(processMap[index(args[1])], args[2:]...)
		}
	})
	processMap[24] = turnstileFunc(func(args ...any) {
		if tv, ok := value(args[1]).(string); ok {
			if nv, ok := value(args[2]).(string); ok {
				processMap[index(args[0])] = tv + "." + nv
			}
		}
	})
	for _, raw := range tokenList {
		items, ok := raw.([]any)
		if !ok || len(items) == 0 {
			continue
		}
		fn, ok := processMap[index(items[0])].(turnstileFunc)
		if !ok {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			fn(items[1:]...)
		}()
	}
	return result
}
