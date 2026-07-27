package openrouter

// Model is a single entry from GET /models.
type Model struct {
	ID            string       `json:"id"`
	CanonicalSlug string       `json:"canonical_slug"`
	Name          string       `json:"name"`
	Created       int64        `json:"created"`
	ContextLength int64        `json:"context_length"`
	Architecture  Architecture `json:"architecture"`
	Pricing       Pricing      `json:"pricing"`
}

// Architecture describes what modalities a model accepts and returns.
type Architecture struct {
	Modality         string   `json:"modality"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
}

// Pricing holds USD-per-token prices as decimal strings, exactly as
// OpenRouter reports them (never converted to float except transiently for
// comparison, to avoid precision loss).
type Pricing struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

// Endpoint is a single provider offering from GET
// /models/{author}/{slug}/endpoints.
type Endpoint struct {
	Name         string  `json:"name"`
	ProviderName string  `json:"provider_name"`
	Pricing      Pricing `json:"pricing"`
	// Status is negative for degraded/deprecated providers, 0 for normal.
	Status int `json:"status"`
}

type modelsResponse struct {
	Data []Model `json:"data"`
}

type endpointsResponse struct {
	Data struct {
		Endpoints []Endpoint `json:"endpoints"`
	} `json:"data"`
}
