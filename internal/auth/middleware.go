package auth

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type contextKey string

const userIDKey contextKey = "user_id"

func Middleware() func(http.Handler) http.Handler {
	issuer := os.Getenv("OIDC_ISSUER") // e.g. http://auth.ticketly.com:8080/realms/event-ticketing
	if issuer == "" {
		panic("OIDC_ISSUER env var not set")
	}

	// Setup provider
	provider, err := oidc.NewProvider(context.Background(), issuer)
	if err != nil {
		panic(fmt.Sprintf("Failed to create OIDC provider: %v", err))
	}

	// Verifier (SkipClientIDCheck → no client ID required)
	verifier := provider.Verifier(&oidc.Config{
		SkipClientIDCheck: true,
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing Authorization header", http.StatusUnauthorized)
				return
			}

			// Expect "Bearer <token>"
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				http.Error(w, "invalid Authorization header format", http.StatusUnauthorized)
				return
			}
			rawToken := parts[1]

			// Verify token
			idToken, err := verifier.Verify(r.Context(), rawToken)
			if err != nil {
				http.Error(w, fmt.Sprintf("invalid token: %v", err), http.StatusUnauthorized)
				return
			}

			// Extract claims (we only need sub for now)
			var claims struct {
				Sub string `json:"sub"`
			}
			if err := idToken.Claims(&claims); err != nil {
				http.Error(w, "failed to parse claims", http.StatusUnauthorized)
				return
			}

			// Add user ID into context
			ctx := context.WithValue(r.Context(), userIDKey, claims.Sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Helper to extract user ID in handlers
func UserID(ctx context.Context) string {
	if uid, ok := ctx.Value(userIDKey).(string); ok {
		return uid
	}
	return ""
}
