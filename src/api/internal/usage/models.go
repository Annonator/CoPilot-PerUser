package usage

type MonthlyUsage struct {
	Period         Period         `json:"period"`
	User           User           `json:"user"`
	Totals         UsageTotals    `json:"totals"`
	Budget         *UsageBudget   `json:"budget,omitempty"`
	Models         []ModelUsage   `json:"models"`
	Daily          []DailyUsage   `json:"daily"`
	SourceMetadata SourceMetadata `json:"sourceMetadata"`
}

type Period struct {
	Year  int `json:"year"`
	Month int `json:"month"`
}

type User struct {
	Email       string `json:"email"`
	GitHubLogin string `json:"githubLogin"`
}

type UsageTotals struct {
	IncludedCredits   float64 `json:"includedCredits"`
	AdditionalCredits float64 `json:"additionalCredits"`
	GrossAmount       float64 `json:"grossAmount"`
	AdditionalUsage   float64 `json:"additionalUsage"`
}

type UsageBudget struct {
	MonthlyIncludedCredits float64 `json:"monthlyIncludedCredits"`
}

type ModelUsage struct {
	Model             string  `json:"model"`
	IncludedCredits   float64 `json:"includedCredits"`
	AdditionalCredits float64 `json:"additionalCredits"`
	GrossAmount       float64 `json:"grossAmount"`
	AdditionalUsage   float64 `json:"additionalUsage"`
	PricePerCredit    float64 `json:"pricePerCredit"`
}

type DailyUsage struct {
	Day    string       `json:"day"`
	Models []ModelUsage `json:"models"`
	Totals UsageTotals  `json:"totals"`
}

type SourceMetadata struct {
	Enterprise string `json:"enterprise"`
	Source     string `json:"source"`
	Cached     bool   `json:"cached"`
}
