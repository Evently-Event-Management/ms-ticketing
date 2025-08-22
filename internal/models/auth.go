package models

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type Config struct {
	KeycloakURL   string
	KeycloakRealm string
	ClientID      string
	ClientSecret  string
}

type M2MTokenResponse struct {
	AccessToken string `json:"access_token"`
}
