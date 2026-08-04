package providers

// ProviderMeta holds the canonical definition for a provider:
// its key, display name, environment variable, signup URL, API key prefix,
// discovery behavior, and default API endpoint.
type ProviderMeta struct {
	Key          string // "nvidia"
	Name         string // "NVIDIA NIM"
	EnvVar       string // "NVIDIA_API_KEY"
	SignupURL    string // "https://build.nvidia.com/explore/discover"
	KeyPrefix    string // "nvapi-"
	Discoverable bool   // true if AutoDiscover can probe its /v1/models
	APIURL       string // "https://integrate.api.nvidia.com/v1/chat/completions"
}

// AllProviders is the single source of truth for every provider supported by
// free-ai-router. Adding a new provider only requires one entry here.
var AllProviders = []ProviderMeta{
	{
		Key: "nvidia", Name: "NVIDIA NIM",
		EnvVar: "NVIDIA_API_KEY", SignupURL: "https://build.nvidia.com/explore/discover",
		KeyPrefix: "nvapi-", Discoverable: true,
		APIURL: "https://integrate.api.nvidia.com/v1/chat/completions",
	},
	{
		Key: "groq", Name: "Groq",
		EnvVar: "GROQ_API_KEY", SignupURL: "https://console.groq.com/keys",
		KeyPrefix: "gsk_", Discoverable: true,
		APIURL: "https://api.groq.com/openai/v1/chat/completions",
	},
	{
		Key: "cerebras", Name: "Cerebras",
		EnvVar: "CEREBRAS_API_KEY", SignupURL: "https://cloud.cerebras.ai/",
		KeyPrefix: "cerebras", Discoverable: true,
		APIURL: "https://api.cerebras.net/v1/chat/completions",
	},
	{
		Key: "opencode", Name: "OpenCode",
		EnvVar: "OPENCODE_API_KEY", SignupURL: "https://opencode.ai/",
		KeyPrefix: "oc-", Discoverable: true,
		APIURL: "https://opencode.ai/api/v1/chat/completions",
	},
	{
		Key: "openai-compatible", Name: "OpenAI Compatible",
		EnvVar: "OPENAI_COMPATIBLE_API_KEY", SignupURL: "",
		KeyPrefix: "", Discoverable: false, APIURL: "",
	},
	{
		Key: "ollama", Name: "Ollama",
		EnvVar: "OLLAMA_API_KEY", SignupURL: "https://ollama.com/",
		KeyPrefix: "", Discoverable: false, APIURL: "",
	},
	{
		Key: "openrouter", Name: "OpenRouter",
		EnvVar: "OPENROUTER_API_KEY", SignupURL: "https://openrouter.ai/keys",
		KeyPrefix: "sk-or-", Discoverable: true,
		APIURL: "https://openrouter.ai/api/v1/chat/completions",
	},
	{
		Key: "codestral", Name: "Codestral",
		EnvVar: "CODESTRAL_API_KEY", SignupURL: "https://console.mistral.ai/",
		KeyPrefix: "", Discoverable: false,
		APIURL: "https://api.codestral.com/v1/chat/completions",
	},
	{
		Key: "scaleway", Name: "Scaleway",
		EnvVar: "SCALEWAY_API_KEY", SignupURL: "https://console.scaleway.com/",
		KeyPrefix: "", Discoverable: true,
		APIURL: "https://api.scaleway.com/llm/v1/chat/completions",
	},
	{
		Key: "kilocode", Name: "KiloCode",
		EnvVar: "KILOCODE_API_KEY", SignupURL: "https://kilocode.ai/",
		KeyPrefix: "", Discoverable: true,
		APIURL: "https://kilocode.ai/api/v1/chat/completions",
	},
	{
		Key: "googleai", Name: "Google AI",
		EnvVar: "GOOGLE_API_KEY", SignupURL: "https://aistudio.google.com/apikey",
		KeyPrefix: "AIza", Discoverable: true,
		APIURL: "https://generativelanguage.googleapis.com/v1beta/models",
	},
	{
		Key: "new-api", Name: "New API",
		EnvVar: "NEW_API_API_KEY", SignupURL: "",
		KeyPrefix: "", Discoverable: false, APIURL: "",
	},
	{
		Key: "siliconflow", Name: "SiliconFlow",
		EnvVar: "SILICONFLOW_API_KEY", SignupURL: "https://siliconflow.cn/",
		KeyPrefix: "", Discoverable: true,
		APIURL: "https://api.siliconflow.cn/v1/chat/completions",
	},
	{
		Key: "baidu", Name: "Baidu Qianfan",
		EnvVar: "QIANFAN_API_KEY", SignupURL: "https://console.bce.baidu.com/qianfan/",
		KeyPrefix: "", Discoverable: false,
		APIURL: "",
	},
	{
		Key: "alibabacloud", Name: "Alibaba Cloud",
		EnvVar: "DASHSCOPE_API_KEY", SignupURL: "https://dashscope.aliyun.com/",
		KeyPrefix: "", Discoverable: false,
		APIURL: "",
	},
	{
		Key: "tencent", Name: "Tencent Cloud",
		EnvVar: "TENCENT_CLOUD_API_KEY", SignupURL: "https://console.cloud.tencent.com/hunyuan",
		KeyPrefix: "", Discoverable: false,
		APIURL: "",
	},
	{
		Key: "kuaipao", Name: "KuaiPao",
		EnvVar: "KUAIPAO_API_KEY", SignupURL: "",
		KeyPrefix: "", Discoverable: false, APIURL: "",
	},
}

// providerMetaByKey is built on first access for O(1) lookup.
var providerMetaByKey map[string]ProviderMeta

func init() {
	providerMetaByKey = make(map[string]ProviderMeta, len(AllProviders))
	for _, p := range AllProviders {
		providerMetaByKey[p.Key] = p
	}
}

// GetProviderMeta returns the canonical metadata for a provider by key.
// Returns the zero value if the key is unknown.
func GetProviderMeta(key string) ProviderMeta {
	return providerMetaByKey[key]
}

// GetProviderEnvVar returns the environment variable name for a provider.
func GetProviderEnvVar(key string) string {
	if m, ok := providerMetaByKey[key]; ok {
		return m.EnvVar
	}
	return ""
}

// GetProviderSignupURL returns the signup URL for a provider.
func GetProviderSignupURL(key string) string {
	if m, ok := providerMetaByKey[key]; ok {
		return m.SignupURL
	}
	return ""
}

// GetProviderKeyPrefix returns the expected API key prefix for a provider.
func GetProviderKeyPrefix(key string) string {
	if m, ok := providerMetaByKey[key]; ok {
		return m.KeyPrefix
	}
	return ""
}

// ProviderKeys returns all known provider keys in registration order.
func ProviderKeys() []string {
	keys := make([]string, len(AllProviders))
	for i, p := range AllProviders {
		keys[i] = p.Key
	}
	return keys
}
