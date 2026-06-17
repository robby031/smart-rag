package auth

type SessionManager struct {
	issuer string
}

func NewSessionManager(issuer string) *SessionManager {
	return &SessionManager{issuer: issuer}
}

func ValidateSession(sessionToken string) bool {
	return sessionToken != ""
}

func RefreshAccessToken(sessionToken string) string {
	if !ValidateSession(sessionToken) {
		return ""
	}
	return sessionToken + ":refreshed"
}
