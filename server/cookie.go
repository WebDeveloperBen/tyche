package server

import (
	"net/http"
	"time"
)

type CookieConfig struct {
	Name     string
	Value    string
	Path     string
	Domain   string
	MaxAge   int
	Secure   bool
	HTTPOnly bool
	SameSite http.SameSite
	Expires  time.Time
}

func SetCookie(w http.ResponseWriter, cfg CookieConfig) {
	if cfg.Path == "" {
		cfg.Path = "/"
	}

	c := &http.Cookie{
		Name:     cfg.Name,
		Value:    cfg.Value,
		Path:     cfg.Path,
		Domain:   cfg.Domain,
		MaxAge:   cfg.MaxAge,
		Secure:   cfg.Secure,
		HttpOnly: cfg.HTTPOnly,
		SameSite: cfg.SameSite,
		Expires:  cfg.Expires,
	}

	http.SetCookie(w, c)
}

func SetCookieDefault(w http.ResponseWriter, name, value string) {
	SetCookie(w, CookieConfig{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   3600,
		HTTPOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func GetCookie(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie(name)
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

func GetCookieOr(r *http.Request, name, defaultValue string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return defaultValue
	}
	return cookie.Value
}

func DeleteCookie(w http.ResponseWriter, name string) {
	SetCookie(w, CookieConfig{
		Name:   name,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}
