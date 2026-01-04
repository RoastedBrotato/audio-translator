package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func UpsertKeycloakUser(sub, preferredUsername, email string, emailVerified bool, displayName string) (*User, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return nil, fmt.Errorf("keycloak subject is required")
	}

	tx, err := DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var userID int
	err = tx.QueryRow(`SELECT user_id FROM keycloak_users WHERE keycloak_sub = $1`, sub).Scan(&userID)
	now := time.Now()

	if err == sql.ErrNoRows {
		username, err := generateUniqueUsername(tx, preferredUsername, email, sub)
		if err != nil {
			return nil, err
		}
		if displayName == "" {
			displayName = preferredUsername
		}
		if displayName == "" {
			displayName = username
		}

		var user User
		var emailValue sql.NullString
		if strings.TrimSpace(email) != "" {
			emailValue = sql.NullString{String: email, Valid: true}
		}

		var lastLogin sql.NullTime
		lastLogin.Valid = true
		lastLogin.Time = now

		err = tx.QueryRow(
			`INSERT INTO users (username, display_name, preferred_language, email, email_verified, last_login)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id, username, display_name, preferred_language, email, email_verified, last_login, created_at`,
			username,
			displayName,
			"en",
			emailValue,
			emailVerified,
			now,
		).Scan(
			&user.ID,
			&user.Username,
			&user.DisplayName,
			&user.PreferredLanguage,
			&emailValue,
			&user.EmailVerified,
			&lastLogin,
			&user.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("insert user: %w", err)
		}

		if emailValue.Valid {
			user.Email = emailValue.String
		}
		if lastLogin.Valid {
			user.LastLogin = &lastLogin.Time
		}

		if _, err := tx.Exec(
			`INSERT INTO keycloak_users (user_id, keycloak_sub, preferred_username, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $4)`,
			user.ID,
			sub,
			nullString(preferredUsername),
			now,
		); err != nil {
			return nil, fmt.Errorf("insert keycloak user: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit transaction: %w", err)
		}

		return &user, nil
	}

	if err != nil {
		return nil, fmt.Errorf("lookup keycloak user: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE users
		 SET email = COALESCE(NULLIF($1, ''), email),
		     email_verified = CASE WHEN $1 = '' THEN email_verified ELSE $2 END,
		     last_login = $3,
		     display_name = COALESCE(NULLIF($4, ''), display_name)
		 WHERE id = $5`,
		strings.TrimSpace(email),
		emailVerified,
		now,
		strings.TrimSpace(displayName),
		userID,
	); err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}

	if _, err := tx.Exec(
		`UPDATE keycloak_users
		 SET preferred_username = COALESCE(NULLIF($1, ''), preferred_username),
		     updated_at = $2
		 WHERE keycloak_sub = $3`,
		strings.TrimSpace(preferredUsername),
		now,
		sub,
	); err != nil {
		return nil, fmt.Errorf("update keycloak user: %w", err)
	}

	var user User
	var emailValue sql.NullString
	var lastLogin sql.NullTime
	err = tx.QueryRow(
		`SELECT id, username, display_name, preferred_language, email, email_verified, last_login, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.PreferredLanguage,
		&emailValue,
		&user.EmailVerified,
		&lastLogin,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}

	if emailValue.Valid {
		user.Email = emailValue.String
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return &user, nil
}

func generateUniqueUsername(tx *sql.Tx, preferredUsername, email, sub string) (string, error) {
	base := sanitizeUsername(preferredUsername)
	if base == "" {
		base = sanitizeUsername(emailLocalPart(email))
	}
	if base == "" {
		base = "user"
	}

	maxAttempts := 20
	for i := 0; i < maxAttempts; i++ {
		candidate := base
		if i > 0 {
			candidate = fmt.Sprintf("%s_%d", base, i+1)
		}

		var exists bool
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)`, candidate).Scan(&exists); err != nil {
			return "", fmt.Errorf("check username: %w", err)
		}
		if !exists {
			return candidate, nil
		}
	}

	shortSub := sanitizeUsername(sub)
	if shortSub != "" {
		candidate := fmt.Sprintf("%s_%s", base, shortSub)
		if len(candidate) > 100 {
			candidate = candidate[:100]
		}
		return candidate, nil
	}

	return "", fmt.Errorf("could not generate unique username")
}

func sanitizeUsername(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}

	result := strings.Trim(b.String(), "._-")
	if result == "" {
		return ""
	}
	if len(result) > 90 {
		result = result[:90]
	}
	return result
}

func emailLocalPart(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	parts := strings.SplitN(email, "@", 2)
	return parts[0]
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
