// Package filtering provides DNS content filtering.
package filtering

// ListDefinition describes a built-in blocklist.
type ListDefinition struct {
	ID          string
	Name        string
	URL         string
	Category    string
	Description string
	Paid        bool
}

// Catalog lists the built-in blocklists available for selection.
var Catalog = map[string]ListDefinition{
	"blocklistproject_malware": {
		ID:          "blocklistproject_malware",
		Name:        "Block List Project - Malware",
		URL:         "https://blocklistproject.github.io/Lists/malware.txt",
		Category:    "malware",
		Description: "Hosts associated with malware distribution.",
	},
	"blocklistproject_phishing": {
		ID:          "blocklistproject_phishing",
		Name:        "Block List Project - Phishing",
		URL:         "https://blocklistproject.github.io/Lists/phishing.txt",
		Category:    "phishing",
		Description: "Hosts associated with phishing campaigns.",
	},
	"blocklistproject_scam": {
		ID:          "blocklistproject_scam",
		Name:        "Block List Project - Scam",
		URL:         "https://blocklistproject.github.io/Lists/scam.txt",
		Category:    "scam",
		Description: "Hosts associated with scam activity.",
	},
	"blocklistproject_porn": {
		ID:          "blocklistproject_porn",
		Name:        "Block List Project - Porn",
		URL:         "https://blocklistproject.github.io/Lists/porn.txt",
		Category:    "porn",
		Description: "Hosts associated with adult content.",
	},
	"blocklistproject_ads": {
		ID:          "blocklistproject_ads",
		Name:        "Block List Project - Ads",
		URL:         "https://blocklistproject.github.io/Lists/ads.txt",
		Category:    "ads",
		Description: "Advertising and tracking hosts.",
	},
	"stevenblack_adult": {
		ID:          "stevenblack_adult",
		Name:        "StevenBlack - Porn Only",
		URL:         "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/porn-only/hosts",
		Category:    "porn",
		Description: "Adult content list without ad/tracker blocking.",
	},
	"oisd_basic": {
		ID:          "oisd_basic",
		Name:        "OISD Basic",
		URL:         "https://big.oisd.nl/",
		Category:    "ads",
		Description: "Ad and tracker blocking list.",
	},
	"adguard_dns": {
		ID:          "adguard_dns",
		Name:        "AdGuard DNS Filter",
		URL:         "https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt",
		Category:    "ads",
		Description: "Ad and tracker blocking list.",
	},
	"spamhaus_dbl": {
		ID:          "spamhaus_dbl",
		Name:        "Spamhaus DBL",
		URL:         "https://www.spamhaus.org/blocklists/domain-blocklist/",
		Category:    "malware",
		Description: "Paid domain blocklist from Spamhaus.",
		Paid:        true,
	},
	"surbl": {
		ID:          "surbl",
		Name:        "SURBL",
		URL:         "https://www.surbl.org/",
		Category:    "malware",
		Description: "Paid URL blocklist from SURBL.",
		Paid:        true,
	},
}
