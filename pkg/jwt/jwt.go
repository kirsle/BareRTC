package jwt

import (
	"encoding/json"
	"errors"
	"html/template"
	"time"

	"git.kirsle.net/apps/barertc/pkg/config"
	"github.com/golang-jwt/jwt/v4"
)

// Custom JWT Claims.
type Claims struct {
	// Custom claims.
	IsAdmin    bool   `json:"op,omitempty"`
	VIP        bool   `json:"vip,omitempty"`
	Avatar     string `json:"img,omitempty"`
	ProfileURL string `json:"url,omitempty"`
	Nick       string `json:"nick,omitempty"`
	Emoji      string `json:"emoji,omitempty"`
	Gender     string `json:"gender,omitempty"`
	Rules      Rules  `json:"rules,omitempty"`
	Flair      Flair  `json:"flair,omitempty"`

	// Standard claims. Notes:
	// subject = username
	jwt.RegisteredClaims
}

// Flair adds an optional custom icon and decoration to users on chat (e.g. like VIP but no special features).
type Flair struct {
	// Plain text of the flair, e.g. "Newbie"
	Name string `json:"name,omitempty"`

	// Bulma CSS color class for the flair icon and tag color in Profile Cards.
	// One of: info, success, warning, danger, private
	// Gets composed into class names e.g.: has-text-info, is-info
	Color string `json:"color,omitempty"`

	// FontAwesome CSS classes for the icon.
	// e.g.: "fa fa-star"
	// Default: "fa fa-star"
	Icon string `json:"icon,omitempty"`

	// NoVIP boolean: if set on a user with VIP status, their VIP flair is hidden from display.
	// Other custom Flair properties may still show. If the Flair has only NoVIP set but everything
	// else empty, then no VIP and no custom flair will show.
	NoVIP bool `json:"noVIP,omitempty"`
}

// ToJSON serializes the claims to JavaScript.
func (c Claims) ToJSON() template.JS {
	data, _ := json.Marshal(c)
	return template.JS(data)
}

// ParseAndValidate returns the Claims, a boolean authOK, and any errors.
func ParseAndValidate(tokenStr string) (*Claims, bool, error) {
	// Handle a JWT authentication token.
	var (
		claims = &Claims{}
		authOK bool
	)
	if tokenStr != "" {
		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			return []byte(config.Current.JWT.SecretKey), nil
		})
		if err != nil {
			return nil, false, err
		}

		if parsed, ok := token.Claims.(*Claims); ok && token.Valid {
			claims = parsed
			authOK = true
		} else {
			return nil, false, errors.New("claims did not parse OK")
		}
	}

	return claims, authOK, nil
}

// ReSign will sign a new JWT token for existing claims. The chat server does this to send refreshed tokens
// to the front-end so the server can reboot gracefully, clients reconnect and not be told their auth had
// expired. New token expires after 5 minutes.
func (c Claims) ReSign() (string, error) {
	// Refresh timestamps.
	c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(5 * time.Minute))
	c.IssuedAt = jwt.NewNumericDate(time.Now())
	c.NotBefore = jwt.NewNumericDate(time.Now())

	// Generate the signed token and return it.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	ss, err := token.SignedString([]byte(config.Current.JWT.SecretKey))
	return ss, err
}
