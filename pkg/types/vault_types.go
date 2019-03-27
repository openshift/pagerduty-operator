package types

// VaultData describes a struct that we will use to pass data from vault to other functions
type VaultData struct {
	Namespace  string
	SecretName string
	Path       string
	Property   string
	URL        string
	Token      string
	Mount      string
	Key        string
}
