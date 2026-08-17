package session

import (
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

const cookieName = "reno_session"

// User is the resolved auth record for the current request.
type User struct {
	ID    string
	Email string
	Name  string
}

func FromRequest(app core.App, r *http.Request) *User {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	record, err := app.FindAuthRecordByToken(c.Value, core.TokenTypeAuth)
	if err != nil {
		return nil
	}
	return &User{
		ID:    record.Id,
		Email: record.Email(),
		Name:  record.GetString("name"),
	}
}

// Login validates credentials and returns the token to store in the session cookie.
func Login(app core.App, email, password string) (string, error) {
	record, err := app.FindAuthRecordByEmail("users", email)
	if err != nil {
		return "", errors.New("invalid credentials")
	}
	if !record.ValidatePassword(password) {
		return "", errors.New("invalid credentials")
	}
	return record.NewAuthToken()
}

// Register creates a new user account and returns the session token.
func Register(app core.App, name, email, password string) (string, error) {
	collection, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		return "", err
	}
	record := core.NewRecord(collection)
	record.SetEmail(email)
	record.SetPassword(password)
	record.Set("name", name)
	if err := app.Save(record); err != nil {
		return "", err
	}
	return record.NewAuthToken()
}

// Cookie returns the session cookie to set.
func Cookie(token string) *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 30, // 30 days
	}
}

// ClearedCookie returns a cookie that expires the session.
func ClearedCookie() *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}
