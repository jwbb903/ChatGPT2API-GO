package app

import (
	"os"
	"strings"

	"github.com/bogdanfinn/tls-client/profiles"
)

func upstreamTLSProfile() profiles.ClientProfile {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("CHATGPT2API_TLS_PROFILE")))
	if name == "" {
		// 默认与 Python 版的 Edge 风格请求头保持一致；可通过环境变量切换：chrome_146、chrome_133、chrome_120 等。
		if profile, ok := profiles.MappedTLSClients["edge_101"]; ok {
			return profile
		}
		return profiles.Chrome_120
	}
	if profile, ok := profiles.MappedTLSClients[name]; ok {
		return profile
	}
	normalized := strings.ReplaceAll(name, "-", "_")
	if profile, ok := profiles.MappedTLSClients[normalized]; ok {
		return profile
	}
	return profiles.Chrome_120
}

func upstreamUserAgent() string {
	if ua := strings.TrimSpace(os.Getenv("CHATGPT2API_USER_AGENT")); ua != "" {
		return ua
	}
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
}

func upstreamSecCHUA() string {
	if chua := strings.TrimSpace(os.Getenv("CHATGPT2API_SEC_CH_UA")); chua != "" {
		return chua
	}
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("CHATGPT2API_TLS_PROFILE")))
	version := "143"
	for _, candidate := range []string{"146", "144", "143", "133", "131", "124", "120", "117", "112", "111", "110", "109", "108", "107", "106", "105", "104", "103"} {
		if strings.Contains(profile, candidate) {
			version = candidate
			break
		}
	}
	if strings.Contains(profile, "edge") || profile == "" {
		return `"Microsoft Edge";v="` + version + `", "Chromium";v="` + version + `", "Not A(Brand";v="24"`
	}
	return `"Google Chrome";v="` + version + `", "Chromium";v="` + version + `", "Not_A Brand";v="24"`
}
