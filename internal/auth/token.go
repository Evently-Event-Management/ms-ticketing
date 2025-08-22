package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// ExtractTokenFromRequest extracts a JWT token from an HTTP request's Authorization header
func ExtractTokenFromRequest(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("authorization header is missing")
	}

	// Bearer token format: "Bearer {token}"
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", errors.New("authorization header format must be 'Bearer {token}'")
	}

	return parts[1], nil
}

// ExtractUserIDFromJWT extracts the user ID from a JWT token
// This function parses the JWT and extracts the 'sub' claim which contains the user ID
func ExtractUserIDFromJWT(tokenString string) (string, error) {
	if tokenString == "" {
		return "", errors.New("empty token")
	}

	// Parse the JWT without validating the signature
	// In a production environment, you should validate the signature
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract claims from token
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims")
	}

	// Extract the subject claim which contains the user ID
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return "", errors.New("subject claim not found in token")
	}

	return sub, nil
}
